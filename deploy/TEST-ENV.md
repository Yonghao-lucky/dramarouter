# 隔离测试环境

与生产 `dramarouter-us` 完全分离的第二套环境，用于新功能开发、压测和脚本验证。
**生产容器、生产数据库、生产 nginx 站点都不受本环境影响。**

## 端口与容器

| 用途 | 容器 | 端口 |
| --- | --- | --- |
| 应用 a（master） | `dramarouter-test-a` | `127.0.0.1:14110` |
| 应用 b（slave） | `dramarouter-test-b` | `127.0.0.1:14111` |
| PostgreSQL | `dramarouter-test-pg` | `127.0.0.1:15434` |
| Redis | `dramarouter-test-redis` | `127.0.0.1:16381` |

| 内容 | 路径 |
| --- | --- |
| 编排文件 | `/opt/1panel/docker/compose/dramarouter-test/compose.yml` |
| 环境变量 | 同目录 `.env`（600 权限，测试专用密钥，**与生产不共享**） |
| 数据 / 日志 / 快照 | `/opt/1panel/apps/local/dramarouter-test/{a,b}-{data,logs,snapshots}` |

数据卷是独立的 `dramarouter-test-pg-data` / `dramarouter-test-redis-data`，与生产的
`dramarouter-us-pg-data` 不共用。

## 生产环境的硬约束

`app-b` 使用 `NODE_TYPE=slave`，因此只有 `app-a` 执行 `AutoMigrate`。这是测试环境的
刻意安排：两台同时迁移会在启动时争抢 PostgreSQL 表锁。

**绝不要让测试实例连生产数据库。** 启动即迁移，新表会直接落到生产库上。

## 从生产备份灌入测试数据

```bash
# 1) 起测试库
cd /opt/1panel/docker/compose/dramarouter-test && docker compose up -d pg redis
docker exec dramarouter-test-pg pg_isready -U dramarouter -d dramarouter

# 2) 导入最近一份生产备份（只读生产备份文件，不连生产库）
zcat /opt/dramarouter-backups/dramarouter-$(date +%F)-*.sql.gz \
  | docker exec -i dramarouter-test-pg psql -U dramarouter -d dramarouter

# 3) 起应用
docker compose up -d app-a app-b
```

导入后测试库里是生产的**副本**，可以随意改价、充值、造测试数据。

## 构建与部署测试镜像

镜像标签与生产隔离，不会覆盖 `dramarouter:latest`：

```bash
cd /root/dramarouter
docker build -t dramarouter-test:$(git rev-parse --short HEAD) -t dramarouter-test:latest .

cd /opt/1panel/docker/compose/dramarouter-test
docker compose up -d --no-deps --force-recreate app-a
# 确认 14110 健康后再动 b
curl -sf http://127.0.0.1:14110/api/status && docker compose up -d --no-deps --force-recreate app-b
```

## 验收清单

| 项 | 检查方式 |
| --- | --- |
| 后端健康 | `curl -s http://127.0.0.1:14110/api/status`、`14111` |
| 新表已创建 | `docker exec dramarouter-test-pg psql -U dramarouter -d dramarouter -c "\dt reseller_*"` |
| 结算台账 | 后台 → 经销商管理 → 管理某经销商 → 登记收款 → 确认到账 |
| 经销商端 | 经销商账号登录 → 额度记录 / 结算台账 / 客户数据 / 定价模板 / 预警 / 审计 |
| 登录可用 | 测试环境 `SESSION_COOKIE_SECURE=false`，走 http 可直接登录 |

## 销毁

```bash
cd /opt/1panel/docker/compose/dramarouter-test
docker compose down            # 保留数据卷
docker compose down -v         # 连数据卷一起删
```

`down -v` 只删除 `dramarouter-test-pg-data` / `dramarouter-test-redis-data`，
不会触及生产的 `dramarouter-us-pg-data`。
