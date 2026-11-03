#!/bin/bash
# 健康看门狗：读取容器的 Docker healthcheck 结果，把 unhealthy 的后端从
# nginx upstream 里标为 down，恢复 healthy 后再挂回。只有配置真的变化才 reload。
# 建议由 cron 每分钟执行一次: * * * * * /opt/dramarouter-secrets/health-watchdog.sh
set -euo pipefail

UP=/opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf
CTR=1Panel-openresty-RqEz
NAME=dramarouter_us
PORTS=(14010 14011)
CONTAINERS=(dramarouter-us-a dramarouter-us-b)

health_of() {
  # starting 尚未完成首次检查，按可用处理，避免刚启动就被摘掉。
  local status
  status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$1" 2>/dev/null || echo missing)
  echo "$status"
}

down_ports=()
for i in "${!CONTAINERS[@]}"; do
  status=$(health_of "${CONTAINERS[$i]}")
  case "$status" in
    unhealthy)
      down_ports+=("${PORTS[$i]}")
      ;;
    missing|none)
      # 容器不存在或没有 healthcheck：保持现状，不主动摘除。
      ;;
  esac
done

# 两个后端都不可用时全部保持挂载：宁可让 nginx 自己重试，也不要造成整站 502。
if [ "${#down_ports[@]}" -ge "${#PORTS[@]}" ]; then
  down_ports=()
fi

render() {
  local port line
  echo "upstream $NAME {"
  for port in "${PORTS[@]}"; do
    line="    server 127.0.0.1:$port"
    if printf '%s\n' "${down_ports[@]:-}" | grep -qx "$port"; then
      line+=" down;"
    else
      line+=" max_fails=2 fail_timeout=10s;"
    fi
    echo "$line"
  done
  echo "    keepalive 64;"
  echo "}"
}

new_conf=$(render)
old_conf=$(cat "$UP" 2>/dev/null || true)
if [ "$new_conf" = "$old_conf" ]; then
  exit 0
fi

printf '%s\n' "$new_conf" > "$UP.tmp"
mv "$UP.tmp" "$UP"
if ! docker exec "$CTR" nginx -t >/dev/null 2>&1; then
  printf '%s\n' "$old_conf" > "$UP"
  echo "nginx -t 失败，已回滚 upstream 配置" >&2
  exit 1
fi
docker exec "$CTR" nginx -s reload >/dev/null
echo "upstream 已更新: down=${down_ports[*]:-none}"
