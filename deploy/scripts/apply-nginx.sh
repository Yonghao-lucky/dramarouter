#!/bin/bash
# 单机部署：恢复美国服务器的 upstream 与两个站点的反向代理配置。
# 1Panel 改动站点会覆盖 proxy/root.conf，点过任何站点相关按钮后重跑本脚本即可复原。
# 用法: apply-nginx.sh
set -euo pipefail

UP=/opt/1panel/www/conf.d/00-dramarouter-us-upstream.conf
CTR=1Panel-openresty-RqEz
SITES_ROOT=/opt/1panel/www/sites

cat > "$UP" <<'CONF'
upstream dramarouter_us {
    # AI 流式请求单条可跑十几分钟，不能把"读超时"当成节点故障，
    # 否则两节点互相顶掉、全部请求变成 502。
    # max_fails=0 关闭失败计数，502 只来自连接层面的真实失败。
    server 127.0.0.1:14010 max_fails=0;
    server 127.0.0.1:14011 max_fails=0;
    keepalive 512;
}
CONF

write_site() {  # $1 = 站点目录名, $2 = 请求体上限
  local site="$1" body="$2"
  mkdir -p "$SITES_ROOT/$site/proxy"
  cat > "$SITES_ROOT/$site/proxy/root.conf" <<CONF
location ^~ / {
    proxy_pass http://dramarouter_us;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_connect_timeout 30s;
    proxy_send_timeout 3600s;
    proxy_read_timeout 3600s;
    send_timeout 3600s;
    client_max_body_size $body;
    client_body_timeout 1200s;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_cache off;
    # 只对连接类错误换节点：上游把连接拒了/断了才算真实失败。
    # 不要带 timeout，长流式请求超时换节点会导致同一请求被上游执行两次。
    proxy_next_upstream error;
    proxy_next_upstream_tries 2;
    add_header X-Accel-Buffering no always;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_set_header X-Forwarded-Port \$server_port;
    add_header Strict-Transport-Security "max-age=31536000";
}
CONF
}

write_site realmdrama.com 200m
write_site studio.realmdrama.com 600m

docker exec "$CTR" nginx -t 2>&1 | tail -2
docker exec "$CTR" nginx -s reload
echo "== 美国 nginx 配置已复原（upstream + realmdrama.com + studio.realmdrama.com）"
