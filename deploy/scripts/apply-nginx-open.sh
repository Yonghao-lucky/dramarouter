#!/bin/bash
# realmrouter.com（美国新站）：恢复 upstream 与站点反向代理配置。
# 1Panel 改动站点会覆盖 proxy/root.conf，点过任何站点相关按钮后重跑本脚本即可复原。
# 用法: apply-nginx-open.sh
set -euo pipefail

UP=/opt/1panel/www/conf.d/00-dramarouter-open-upstream.conf
CTR=1Panel-openresty-RqEz
SITE=/opt/1panel/www/sites/realmrouter.com

cat > "$UP" <<'CONF'
upstream dramarouter_open {
    # 与生产站一致：长流式请求不能按读超时记账，真实故障交给健康看门狗。
    server 127.0.0.1:14110 max_fails=0;
    server 127.0.0.1:14111 max_fails=0;
    keepalive 512;
}
CONF

mkdir -p "$SITE/proxy"
cat > "$SITE/proxy/root.conf" <<'CONF'
location ^~ / {
    proxy_pass http://dramarouter_open;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_connect_timeout 30s;
    proxy_send_timeout 3600s;
    proxy_read_timeout 3600s;
    send_timeout 3600s;
    client_max_body_size 140m;
    client_body_timeout 1200s;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_cache off;
    # 只对连接类错误换节点：上游把连接拒了/断了才算真实失败。
    # 不要带 timeout，长流式请求超时换节点会导致同一请求被上游执行两次。
    proxy_next_upstream error;
    proxy_next_upstream_tries 2;
    add_header X-Accel-Buffering no always;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Port $server_port;
    add_header Strict-Transport-Security "max-age=31536000";
}
CONF

docker exec "$CTR" nginx -t 2>&1 | tail -2
docker exec "$CTR" nginx -s reload
echo "== 新站 nginx 配置已复原（upstream + realmrouter.com）"
