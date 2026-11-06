#!/bin/bash
# 单机滚动更新：一次只换一个容器，另一个始终在线，客户无感。
# 用法: rollout.sh [镜像名]   默认 dramarouter:latest
#
# 任一步失败会自动回滚：恢复双后端 upstream，并把本次已重建的服务退回更新前镜像。
# 环境变量:
#   DRAIN_SECONDS  摘除后端后等待旧连接排空的秒数，默认 10
#   FORCE=1        两台不全是 healthy 时仍强制滚动（会放弃零停机保证）
set -euo pipefail

IMG="${1:-dramarouter:latest}"
PROJ=/opt/1panel/docker/compose/dramarouter-us
UP=/opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf
CTR=1Panel-openresty-RqEz
DRAIN_SECONDS="${DRAIN_SECONDS:-10}"

SERVICES=(app-a app-b)
CONTAINERS=(dramarouter-us-a dramarouter-us-b)
PORTS=(14010 14011)

# 本次运行中已重建的服务，失败时倒序回滚
ROLLED=()
# 更新前 dramarouter:latest 指向的镜像 ID，失败时把标签指回去
PREV_IMAGE=""

docker image inspect "$IMG" >/dev/null 2>&1 || { echo "镜像不存在: $IMG"; exit 1; }

# 以当前运行中的容器为准记录回滚镜像，比读标签更可靠（标签可能已被覆盖）
PREV_IMAGE=$(docker inspect --format '{{.Image}}' "${CONTAINERS[0]}" 2>/dev/null || true)
if [ -z "$PREV_IMAGE" ]; then
  PREV_IMAGE=$(docker inspect --format '{{.Id}}' dramarouter:latest 2>/dev/null || true)
fi

preflight() {
  local i st bad=0
  echo "=== 预检：两台都 healthy 才具备零停机条件"
  for i in 0 1; do
    st=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "${CONTAINERS[$i]}" 2>/dev/null || echo missing)
    echo "  ${CONTAINERS[$i]}: $st"
    if [ "$st" != "healthy" ]; then bad=1; fi
  done
  if [ "$bad" -eq 1 ] && [ "${FORCE:-0}" != "1" ]; then
    echo "拒绝滚动：请先把两台恢复到 healthy，或确认无零停机要求后设 FORCE=1" >&2
    exit 2
  fi
}

healthy_ports() {  # 输出当前真能响应 /api/status 的端口，逗号分隔
  local p out=""
  for p in "${PORTS[@]}"; do
    if curl -sf -o /dev/null --max-time 3 "http://127.0.0.1:$p/api/status"; then out+="$p,"; fi
  done
  echo "${out%,}"
}

set_upstream() {  # $1 = 逗号分隔的保持服务端口；留空 = 双后端都挂载
  local keep="${1:-}" a b backup
  # max_fails=0：AI 流式请求的长耗时不能被当成节点故障，
  # 否则发布期间摘掉一台后，剩下一台的读超时会把最后可用节点也顶掉。
  if [ -z "$keep" ] || [[ ",$keep," == *",14010,"* ]]; then a="max_fails=0;"; else a="down;"; fi
  if [ -z "$keep" ] || [[ ",$keep," == *",14011,"* ]]; then b="max_fails=0;"; else b="down;"; fi

  backup=$(mktemp)
  cp "$UP" "$backup" 2>/dev/null || true
  cat > "$UP" <<EOF
upstream dramarouter_us {
    server 127.0.0.1:14010 $a
    server 127.0.0.1:14011 $b
    keepalive 512;
}
EOF
  if ! docker exec "$CTR" nginx -t >/dev/null 2>&1; then
    cp "$backup" "$UP"
    rm -f "$backup"
    echo "nginx -t 失败，upstream 已恢复原样" >&2
    return 1
  fi
  rm -f "$backup"
  docker exec "$CTR" nginx -s reload
}

wait_healthy() {  # $1 = 端口
  local port="$1" i
  for i in $(seq 1 80); do
    if curl -sf -o /dev/null --max-time 5 "http://127.0.0.1:$port/api/status"; then
      echo "healthy: $port"
      return 0
    fi
    sleep 3
  done
  echo "FAILED health check on $port" >&2
  return 1
}

idx_of() {  # $1 = 服务名
  local j
  for j in 0 1; do
    if [ "${SERVICES[$j]}" = "$1" ]; then echo "$j"; return 0; fi
  done
  return 1
}

roll_one() {  # $1 = 服务下标
  local i="$1" svc="${SERVICES[$1]}" port="${PORTS[$1]}" peer
  peer="${PORTS[$(( (i + 1) % 2 ))]}"
  echo "=== 更新 $svc ($port)，流量只留在 $peer ==="
  set_upstream "$peer"
  sleep "$DRAIN_SECONDS"
  ROLLED+=("$svc")
  ( cd "$PROJ" && docker compose stop "$svc" && docker compose up -d --no-deps --force-recreate "$svc" )
  wait_healthy "$port"
  # 更新完的这台重新加入，与另一台一起接流量
  set_upstream "$(healthy_ports)"
  echo "=== $svc 已恢复 ==="
}

rollback() {
  local status=$?
  trap - EXIT ERR
  if [ "$status" -eq 0 ]; then exit 0; fi

  echo "!!! 滚动更新失败（退出码 $status），开始自动回滚" >&2
  # 只挂当前真能响应的端口，避免把流量打到正在重建的容器
  local alive
  alive=$(healthy_ports)
  set_upstream "$alive" || true
  echo "当前健康后端: ${alive:-无}" >&2

  if [ -n "$PREV_IMAGE" ]; then
    docker tag "$PREV_IMAGE" dramarouter:latest
    echo "dramarouter:latest 已指回更新前的镜像：$PREV_IMAGE" >&2
  fi

  local i svc j port peer
  for (( i=${#ROLLED[@]}-1; i>=0; i-- )); do
    svc="${ROLLED[$i]}"
    j=$(idx_of "$svc")
    port="${PORTS[$j]}"
    peer=$( { healthy_ports | tr ',' '\n' | grep -vx "$port" | paste -sd, -; } || true )
    if [ -z "$peer" ]; then
      echo "!!! 没有可用后端承接 $svc 回滚，停止自动回滚，请人工处理" >&2
      break
    fi
    echo "--- 回滚 $svc ($port)，流量留在 $peer" >&2
    if ! set_upstream "$peer"; then break; fi
    sleep "$DRAIN_SECONDS"
    if ! ( cd "$PROJ" && docker compose stop "$svc" && docker compose up -d --no-deps --force-recreate "$svc" ); then
      echo "!!! $svc 回滚重建失败，请人工处理" >&2
      break
    fi
    if ! wait_healthy "$port"; then
      echo "!!! $svc 回滚后仍不健康，请人工处理" >&2
      break
    fi
    alive=$(healthy_ports)
    set_upstream "$alive" || true
  done

  alive=$(healthy_ports)
  set_upstream "$alive" || true
  echo "!!! 自动回滚结束，upstream 已挂载当前健康后端: ${alive:-无}；如仍异常按 deploy/DEPLOYMENT.md 处理" >&2
  exit 1
}

preflight

if [ "$IMG" != "dramarouter:latest" ]; then
  # 保留上一版用于回滚（PREV_IMAGE 已在启动时从运行中的容器读取），再打 latest 标签
  if [ -n "$PREV_IMAGE" ]; then docker tag "$PREV_IMAGE" dramarouter:prev 2>/dev/null || true; fi
  docker tag "$IMG" dramarouter:latest
  echo "已把 $IMG 标记为 dramarouter:latest（回滚点 dramarouter:prev）"
fi

trap rollback EXIT
trap 'exit 1' ERR

roll_one 0
roll_one 1

set_upstream ""
trap - EXIT ERR
echo "=== 滚动更新完成，双后端已恢复 ==="
