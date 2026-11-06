# Dramarouter 部署与运维手册（单机版）

最后更新：2026-11-03 ｜ 架构：美国单机（香港已退役，仅作冷备份保留）

本文记录当前生产环境的真实架构、日常更新、回滚、备份与排障方法。所有路径、端口、容器名均为美国服务器上的实际值。

---

## 一、架构总览

生产环境只有一台机器，数据库、Redis、应用全部在本机，不存在跨机房依赖。

```text
                   同一个 sk-xxx
                        │
        ┌───────────────┴───────────────┐
        ▼                               ▼
 https://realmdrama.com        https://studio.realmdrama.com
        │                               │
        └───────────────┬───────────────┘
                        ▼
              美国 184.164.79.210
              1Panel (72C/125G/880G)
                        │
               openresty :443（容器 1Panel-openresty-RqEz）
                        │ /
                        ▼
              upstream dramarouter_us
                ├─ 127.0.0.1:14010
                └─ 127.0.0.1:14011
                        │
       dramarouter-us-a / dramarouter-us-b
              NODE_TYPE=master
                        │
        ┌───────────────┴───────────────┐
        ▼                               ▼
 PostgreSQL 15 主库                  Redis 7
  容器 dramarouter-us-pg              容器 dramarouter-us-redis
  127.0.0.1:15433                    127.0.0.1:16380
  docker 内网 pg-standby:5432         172.17.0.1:16380
                                      maxmemory 6gb, noeviction
```

Docker 网桥 `192.168.240.1/20`，因此 `TRUSTED_PROXIES` 必须是 `127.0.0.1,192.168.240.1`，否则所有客户会被算成同一个 IP 触发全站 429。

### 关键数据流

| 内容 | 存在哪 | 谁读 |
| --- | --- | --- |
| 用户、令牌、余额、订阅 | 本机 PostgreSQL | 两个应用容器 |
| 渠道与上游密钥 | 本机 PostgreSQL | 每 `SYNC_FREQUENCY`=60 秒同步进内存 |
| 限流计数、令牌缓存 | 本机 Redis | 本机 |
| 请求快照 | 各容器自己的 `*-snapshots` | 本机 |
| 视频临时文件 | `/opt/1panel/apps/local/dramarouter/video-tmp` | 本机 |
| 大请求体落盘缓存 | 各容器 `/data/new-api-body-cache` | 本机 |

### 端口与容器对照

| 用途 | 容器 | 端口 |
| --- | --- | --- |
| 应用 a | `dramarouter-us-a` | `127.0.0.1:14010` → 容器 3000 |
| 应用 b | `dramarouter-us-b` | `127.0.0.1:14011` → 容器 3000 |
| PostgreSQL | `dramarouter-us-pg` | `127.0.0.1:15433`；docker 内网 `pg-standby:5432` |
| Redis | `dramarouter-us-redis` | `127.0.0.1:16380`、`172.17.0.1:16380` |
| Nginx | `1Panel-openresty-RqEz` | 80 / 443 |

| 配置 | 路径 |
| --- | --- |
| 编排文件 | `/opt/1panel/docker/compose/dramarouter-us/compose.yml` |
| 编排变量 | 同目录 `.env`（600 权限，**不入库**） |
| upstream 定义 | `/opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf` |
| 站点代理 | `/opt/1panel/www/sites/<域名>/proxy/root.conf` |
| 数据 / 日志 / 快照 | `/opt/1panel/apps/local/dramarouter/{a,b}-{data,logs,snapshots}` |
| 运维脚本 | `/opt/dramarouter-secrets/`（运行时副本）、仓库 `deploy/scripts/`（版本管理） |
| 源码 | `/root/dramarouter`（本仓库）、`/root/new-api`、`/root/new-realm-api` |

### 本机其他业务

这台机器上还有约 40 个别的容器，`api.realmcompute.top`、`miaosha.realmcompute.top`、`router.realmcompute.top` 等站点。**新部署只使用上表列出的容器、端口和目录，其余一律不动。**

---

## 二、日常更新流程

镜像在**本机**构建（72 核）。无需跨机传镜像。

```text
① 拉代码
   cd /root/dramarouter && git pull

② 构建（构建上下文是仓库根目录）
   docker build -t dramarouter:<提交sha> -t dramarouter:next .

③ 滚动更新（脚本会自动打 latest 标签并保留 prev 回滚点）
   /opt/dramarouter-secrets/rollout.sh --site us dramarouter:next
```

更新 open 站（realmrouter.com）时把镜像和站点一起换，生产站不受影响：

```text
docker build -t dramarouter-open:next .
/opt/dramarouter-secrets/rollout.sh --site open dramarouter-open:next
```

`--site` 决定脚本操作哪一套容器和哪一份 upstream，两个站点除名字、端口、目录外流程完全一致：

| `--site` | 域名 | 容器 | 端口 | upstream 文件 | 默认镜像 |
| --- | --- | --- | --- | --- | --- |
| `us`（默认） | realmdrama.com | dramarouter-us-a/-b | 14010/14011 | `00-dramarouter-us-upstream.conf` | `dramarouter:latest` |
| `open` | realmrouter.com | dramarouter-open-a/-b | 14110/14111 | `00-dramarouter-open-upstream.conf` | `dramarouter-open:latest` |

**不带 `--site` 时默认为 `us`**，与旧调用方式兼容。滚动前脚本会确认目标 upstream 与 compose 文件存在，写错站点名会直接退出而不是改到另一套配置。

### 滚动更新做了什么

```text
⓪ 预检：两台都 healthy（否则拒绝执行，除非 FORCE=1）
① 改 upstream：把要更新的容器标 down，新请求只进另一个
② 等 DRAIN_SECONDS（默认 10 秒）让旧连接排空
③ docker compose stop + up -d --no-deps --force-recreate 该容器
④ 轮询 http://127.0.0.1:<端口>/api/status 直到健康
⑤ 把已就绪的这台挂回 upstream（此时又是双后端）
⑥ 对另一个容器重复 ①-⑤
```

客户无感：Nginx 不向 `down` 的节点发新请求，已建立的连接继续跑；容器收到 SIGTERM 后先等手上请求结束再退出（`SHUTDOWN_TIMEOUT_SECONDS=540`，Docker 宽限期 `stop_grace_period=600s`）。退出前还会把批量更新器里缓存的用量增量落库（`model.StopBatchUpdater`），重启不再丢最后 5 秒的配额。

### 失败自动回滚

任何一步失败（新镜像起不来、健康检查 4 分钟未通过、nginx 配置校验失败）都会自动回滚：

```text
① 把 dramarouter:latest 指回更新前运行中的镜像
② upstream 只挂当前真能响应的端口，避免打到重建中的容器
③ 倒序对本次已更新的服务重建，每次重建前把流量留在健康的那台
④ 结束后 upstream 挂载当时健康的全部后端，退出码 1
```

回滚后仍需人工确认日志，但**不会出现只剩单后端裸奔**。

可用环境变量：

| 变量 | 默认 | 用途 |
| --- | --- | --- |
| `DRAIN_SECONDS` | `10` | 摘除后端后等待排空秒数；流量大或大请求多时调大 |
| `FORCE` | 未设 | `1` 时跳过预检。只在明确接受停机风险时使用 |

预检不通过（例如一台已经不健康）时脚本退出码为 2，不会动任何容器。

### 有数据库结构变更的版本

新版可能带 `AutoMigrate`。规则：

1. 只更新 `app-a`，启动后确认日志出现 `database migration completed`。
2. 再更新 `app-b`。
3. 本仓库的迁移是幂等 `AutoMigrate`，但变更较大的版本仍建议先跑一次 `backup-pg.sh`。

```bash
docker logs --since 5m dramarouter-us-a 2>&1 | grep -i migrat
```

### 更新后必做检查

```bash
# 两个容器都是新镜像且 healthy
docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep dramarouter-us

# 两个域名 200
for d in realmdrama.com studio.realmdrama.com; do echo $d $(curl -s -o /dev/null -w '%{http_code}' https://$d/api/status); done

# 真实调用（换成当前可用的 key）
curl -s https://realmdrama.com/v1/chat/completions \
  -H "Authorization: Bearer sk-xxx" -H 'Content-Type: application/json' \
  -d '{"model":"kimi-k3","messages":[{"role":"user","content":"hi"}],"max_tokens":20}'
```

---

## 三、回滚

`rollout.sh` 打标签时会自动把上一版保存为 `<镜像>:prev`（us 站是 `dramarouter:prev`，open 站是 `dramarouter-open:prev`）。

```bash
docker tag dramarouter:prev dramarouter:latest
/opt/dramarouter-secrets/rollout.sh --site us
```

回滚 open 站同理，换成 `dramarouter-open:prev` 和 `--site open`。

**回滚镜像不会回滚数据库结构。** 新版执行过 `AutoMigrate` 后，旧版代码面对新表结构通常能跑（多出没用的列），但若新版做过删列或改语义，回滚前先在测试库验证；真正危险的版本先备份数据库。

---

## 四、只改配置、不改代码

配置分两类，改完都**不需要重新构建镜像**。

### 4.1 改环境变量（连接数、超时、限流、密钥等）

```bash
vi /opt/1panel/docker/compose/dramarouter-us/compose.yml   # 或改动量小的 .env
cd /opt/1panel/docker/compose/dramarouter-us
docker compose up -d --no-deps --force-recreate app-a app-b
```

**不要一次重建两个**——会有约 60 秒两个容器都不在线。逐个重建并把另一个从 upstream 摘掉，或直接跑 `rollout.sh`。

### 4.2 改 Nginx（站点、超时、请求体大小）

1Panel 里改站点会覆盖 `proxy/root.conf`。**在 1Panel 点过任何站点相关按钮后，重跑恢复脚本**：

```bash
/opt/dramarouter-secrets/apply-nginx.sh
```

脚本会重写 upstream 和两个站点的 `root.conf`：

| 站点 | 请求体上限 |
| --- | --- |
| `realmdrama.com` | `client_max_body_size 140m` |
| `studio.realmdrama.com` | `client_max_body_size 500m` |

改完都会 `nginx -t` + reload，失败会留下错误输出、不会破坏现有配置。

### 4.3 改业务配置（渠道、模型定价、分组、key）

全部在后台界面操作，写的是本机数据库，两个容器一分钟内自动同步，不需要重启。

---

## 五、备份（P0，必须做）

数据库目前没有任何定时备份。`backup-pg.sh` 每天从主库导出并保留 14 天。

```bash
# 手动跑一次
/opt/dramarouter-secrets/backup-pg.sh

# 装成每日任务（04:30）
echo '30 4 * * * /opt/dramarouter-secrets/backup-pg.sh >> /var/log/dramarouter-backup.log 2>&1' \
  | crontab -
```

备份目录 `/opt/dramarouter-backups`，文件名 `dramarouter-YYYY-MM-DD-HHMM.sql.gz`。脚本会校验 gzip 完整性和数据段，异常时非零退出。

**恢复**：

```bash
zcat /opt/dramarouter-backups/dramarouter-2026-11-04-0430.sql.gz \
  | docker exec -i dramarouter-us-pg psql -U dramarouter -d dramarouter
```

> 演练提示：备份没恢复过一次就等于没有备份。建议每季度在临时库验证一次。

---

## 六、故障处理

### 6.1 单个容器异常

`restart: unless-stopped` 会自动重启。反复重启时看日志：

```bash
docker logs --tail 100 dramarouter-us-a
```

常见原因：数据库连不上、`SESSION_COOKIE_SECURE=true` 但缺 `SESSION_COOKIE_TRUSTED_URL`、Redis 认证失败。

### 6.2 数据库不可用

单机部署没有热备，数据库故障即全站不可用。恢复顺序：

```bash
# 1) 确认容器状态
docker ps -a --filter name=dramarouter-us-pg
docker logs --tail 100 dramarouter-us-pg

# 2) 从最近备份恢复（会覆盖现有数据，先确认！）
docker exec dramarouter-us-pg psql -U dramarouter -d dramarouter \
  -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
zcat /opt/dramarouter-backups/<最近备份>.sql.gz \
  | docker exec -i dramarouter-us-pg psql -U dramarouter -d dramarouter

# 3) 起应用
cd /opt/1panel/docker/compose/dramarouter-us && docker compose up -d app-a app-b
```

### 6.3 Nginx 全站 502

```bash
# 后端真的活着吗
for p in 14010 14011; do echo $p $(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:$p/api/status); done
# upstream 是不是被摘空了（两个都 down 会造成 502）
cat /opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf
# 复原
/opt/dramarouter-secrets/apply-nginx.sh
```

---

## 七、密钥与一致性约束

两个应用容器必须使用完全相同的值，都在编排目录 `.env` 里（**不入库**）：

```text
PG_PASSWORD          # 数据库密码
REDIS_PASSWORD
SESSION_SECRET       # 登录态签名，变更会导致所有用户退出登录
CRYPTO_SECRET        # 加密上游密钥，变更会导致已保存的渠道密钥无法解密
STORAGE_KEY          # STORAGE_CREDENTIAL_ENCRYPTION_KEY
```

**这些值一旦上线不要更换。**

每个容器的 `SNAPSHOT_SPOOL_DIR` 必须不同（当前 `a-snapshots` / `b-snapshots`），那里有单进程文件锁。

---

## 八、性能相关配置（2026-11-03 调整）

| 项 | 位置 | 值 | 说明 |
| --- | --- | --- | --- |
| `GOMAXPROCS` | compose | `48` | 72 核机器，留余量给 PG 与 nginx |
| `MAX_REQUEST_BODY_MB` | compose | `140` | 与 nginx `client_max_body_size 140m` 对齐；统计解压后大小 |
| 请求体磁盘缓存 | 后台「系统设置 → 性能」 | 阈值 64MB、上限 100GB、路径 `/data` | 大请求体落盘，避免峰值吃掉内存 |
| 性能监控 | 后台 | CPU 90% / 内存 90% / 磁盘 95% | 超阈值会直接拒绝 Relay 请求，调整前先确认不会频繁触发 |
| `STREAMING_TIMEOUT` | compose | `600` | 流式响应**空闲**超时（逐行累加），非总时长 |
| `RELAY_TIMEOUT` | compose | `0` | 上游客户端不限总时长 |

### PostgreSQL 参数

| 参数 | 值 | 说明 |
| --- | --- | --- |
| `shared_buffers` | 8GB | 库仅 1.3GB，可整库常驻内存；**修改需重启** |
| `effective_cache_size` | 64GB | 纯提示，供优化器估算可用缓存 |
| `work_mem` | 32MB | 每连接排序/哈希内存 |
| `maintenance_work_mem` | 1GB | 建索引、VACUUM |

修改方式（`shared_buffers` 需重启容器）：

```bash
docker exec dramarouter-us-pg psql -U dramarouter -d dramarouter \
  -c "ALTER SYSTEM SET shared_buffers = '8GB';"
docker restart dramarouter-us-pg
```

---

## 九、常见坑（都实际踩过）

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 全站一起 429 | `TRUSTED_PROXIES` 没含 Docker 网桥 | 补 `192.168.240.1` 后重建容器 |
| 站点只剩一个容器在接 | 1Panel 改站点覆盖了 `root.conf` | 重跑 `apply-nginx.sh` |
| 大请求 413 | nginx 与 `MAX_REQUEST_BODY_MB` 不一致 | 两处都改，单位都是 MiB |
| 容器启动即退出 | `SESSION_COOKIE_SECURE=true` 缺 `SESSION_COOKIE_TRUSTED_URL` | 补该变量（逗号分隔多个域名） |
| 容器连不上 Redis | 容器里 `127.0.0.1` 不是宿主机 | 用 `172.17.0.1:16380` |
| 容器连不上数据库 | 同上 | 用 docker 服务名 `pg-standby:5432` |
| 已初始化仍要求创建管理员 | `constant.Setup` 是进程内存标志，只在启动时读一次 | 重启两个应用容器 |
| 聚合 key 选不了供应商 | `vendors` 表为空 | 在供应商管理建同名记录 |
| 计价显示 `$0 / 请求` | `QuotaType=1` 但 `ModelPrice` 为空 | 给模型填最低档单价作兜底 |

---

## 十、脚本清单

仓库 `deploy/scripts/` 是版本化副本，部署时拷到 `/opt/dramarouter-secrets/` 并 `chmod +x`。

| 脚本 | 用途 |
| --- | --- |
| `rollout.sh` | 单机滚动更新，`--site us\|open` 选择站点，可传镜像名；自动保留 `<镜像>:prev` |
| `apply-nginx.sh` | 复原 upstream + 两个站点代理配置 |
| `health-watchdog.sh` | 按容器健康状态动态摘除/挂回后端，可配 cron 每分钟执行 |
| `backup-pg.sh` | 每日 `pg_dump` 备份，保留 14 天 |

推荐 cron（线上已装）：

```cron
* * * * * /opt/dramarouter-secrets/health-watchdog.sh
30 4 * * * /opt/dramarouter-secrets/backup-pg.sh >> /var/log/dramarouter-backup.log 2>&1
20 4 * * * find /opt/1panel/apps/local/dramarouter/*-logs -name "oneapi-*.log*" -mtime +14 -delete
```

### 日志轮转

应用在**每次启动**时按启动时间戳新建一个 `oneapi-YYYYMMDDHHMMSS.log`，运行期间追加写同一个文件，自己不做清理。因此光靠 logrotate 的 `rotate`/`maxage` 不够——那两项只统计被轮转过的文件，进程自己新建的旧文件不在其中。

所以要两条一起用：

1. `deploy/logrotate-dramarouter.conf` → 装到 `/etc/logrotate.d/dramarouter`，管**单次运行写太大**（`maxsize 200M`）并压缩。应用以 `O_WRONLY` 持有 fd，必须 `copytruncate`，否则重命名后进程继续写旧 inode。
2. 上面那条 `find ... -mtime +14 -delete` cron，管**重启次数多导致文件堆积**。活跃文件一直在写、`mtime` 始终是新的，不会被误删。

安装：

```bash
cp deploy/logrotate-dramarouter.conf /etc/logrotate.d/dramarouter
chmod 644 /etc/logrotate.d/dramarouter
logrotate -d /etc/logrotate.d/dramarouter   # 调试，只打印不执行
```

每个容器目录当前约 29MB、59 个历史文件；按 14 天保留稳定在 40–60MB 量级。

---

## 十一、开发环境

本机即开发机。三个仓库都在 `/root`：

```bash
/root/dramarouter      # 主仓库（本仓库）
/root/new-api          # 上游 QuantumNous/new-api 参考
/root/new-realm-api    # 衍生项目
```

工具链：

```bash
# 后端：本机没装 Go，用容器构建（与生产一致）
docker run --rm -v /root/dramarouter:/src -v /root/gomodcache:/gomodcache \
  -e GOMODCACHE=/gomodcache -e GOFLAGS=-mod=mod -w /src \
  golang:1.26-bookworm go build -o /tmp/new-api .

# 前端
cd /root/dramarouter/web && bun install && bun run build
```

`web/node_modules`、`web/dist` 均被 gitignore，克隆后需要 `bun install`。

---

## 十二、香港服务器（已退役）

香港 `154.64.249.88` 已停止承接业务，应用容器 `dramarouter-hk-a` / `-b` 已停止且 `restart=no`，避免写回冻结的旧库造成数据分叉。

- 香港 `dramarouter-hk-pg` 保留一份切换时刻的完整数据库副本，作为灾备参考，**不要启动**。
- 香港 nginx 当前配置为**反向代理到美国**（`proxy_pass https://184.164.79.210`），用于兜住 DNS 缓存未刷新的老客户端。观察 1–2 天确认无流量后可下线该站点。
- 停机清理顺序：确认香港站点连续 24 小时无请求 → 停 openresty 站点 → 停止并删除香港 PG/Redis 容器 → 保留磁盘卷至备份验证通过。

---

## 十三、待办（按优先级）

| 优先级 | 事项 | 状态 |
| --- | --- | --- |
| P0 | 每日数据库备份 | 脚本已就绪，需装 cron |
| P1 | Nginx / upstream 配置进版本管理 | 已完成（本目录） |
| P1 | 日志轮转 | 已完成（logrotate + 14 天清理 cron） |
| P2 | `ServerAddress` 与下载域名 | 当前 `https://realmdrama.com`；若要从 studio 域名生成下载链接需调整 |
| P2 | 香港灾备保留周期 | 建议备份跑通并验证恢复后再决定 |
| P3 | 单机无热备 | 如未来需要 HA，再评估流复制 + 人工提升方案 |
