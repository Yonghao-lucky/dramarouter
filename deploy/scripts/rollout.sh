#!/bin/bash
# 单机滚动更新：一次只换一个容器，另一个始终在线，客户无感。
# 用法: rollout.sh [镜像名]   默认 dramarouter:latest
set -euo pipefail
IMG="${1:-dramarouter:latest}"
PROJ=/opt/1panel/docker/compose/dramarouter-us
UP=/opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf
CTR=1Panel-openresty-RqEz

docker image inspect "$IMG" >/dev/null 2>&1 || { echo "镜像不存在: $IMG"; exit 1; }
if [ "$IMG" != "dramarouter:latest" ]; then
  # 保留上一版用于回滚，再打 latest 标签（compose 固定引用 latest）
  PREV=$(docker inspect --format '{{.Id}}' dramarouter:latest 2>/dev/null || true)
  if [ -n "$PREV" ]; then docker tag dramarouter:latest dramarouter:prev 2>/dev/null || true; fi
  docker tag "$IMG" dramarouter:latest
  echo "已把 $IMG 标记为 dramarouter:latest（回滚点 dramarouter:prev）"
fi

set_upstream() {  # $1 = 保持服务的端口，另一个标 down
  local keep="$1"
  cat > "$UP" <<EOF
upstream dramarouter_us {
    server 127.0.0.1:14010 $([ "$keep" = 14011 ] && echo 'down;' || echo 'max_fails=2 fail_timeout=10s;')
    server 127.0.0.1:14011 $([ "$keep" = 14010 ] && echo 'down;' || echo 'max_fails=2 fail_timeout=10s;')
    keepalive 64;
}
EOF
  docker exec "$CTR" nginx -t >/dev/null
  docker exec "$CTR" nginx -s reload
  sleep 3
}

wait_healthy() {  # $1 = 端口
  local port="$1"
  for i in $(seq 1 80); do
    if curl -sf -o /dev/null --max-time 5 "http://127.0.0.1:$port/api/status"; then
      echo "healthy: $port"; return 0
    fi
    sleep 3
  done
  echo "FAILED health check on $port"; return 1
}

roll_one() {  # $1 = a|b   $2 = 端口   $3 = 另一个端口
  local svc="app-$1" port="$2" peer="$3"
  echo "=== 更新 $svc ($port)，流量只留在 $peer ==="
  set_upstream "$peer"
  sleep 5   # 等旧连接排空
  cd "$PROJ"
  docker compose stop "$svc"
  docker compose up -d --no-deps --force-recreate "$svc"
  wait_healthy "$port"
  set_upstream "$port"
  echo "=== $svc 已恢复 ==="
}

roll_one a 14010 14011
roll_one b 14011 14010

# 两个容器都更新完，恢复双后端同时在线
cat > "$UP" <<'CONF'
upstream dramarouter_us {
    server 127.0.0.1:14010 max_fails=2 fail_timeout=10s;
    server 127.0.0.1:14011 max_fails=2 fail_timeout=10s;
    keepalive 64;
}
CONF
docker exec "$CTR" nginx -t >/dev/null
docker exec "$CTR" nginx -s reload
echo "=== 滚动更新完成，双后端已恢复 ==="
