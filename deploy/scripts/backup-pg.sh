#!/bin/bash
# 每日数据库备份。从本机主库导出，gzip 压缩后保留 14 天。
# 用法: backup-pg.sh [目标目录]   默认 /opt/dramarouter-backups
# 建议 cron: 30 4 * * * /opt/dramarouter-secrets/backup-pg.sh
set -euo pipefail

DEST="${1:-/opt/dramarouter-backups}"
CTR=dramarouter-us-pg
KEEP_DAYS=14

mkdir -p "$DEST"
STAMP=$(date +%F-%H%M)
OUT="$DEST/dramarouter-$STAMP.sql.gz"

# 先写临时文件，成功后原子改名，避免中断留下半个备份。
docker exec "$CTR" pg_dump -U dramarouter -d dramarouter --no-owner --no-acl \
  | gzip -6 > "$OUT.tmp"
mv "$OUT.tmp" "$OUT"

# 校验：gzip 完整性 + 至少含一条 COPY 数据段。
# 注意不能用 `zcat | grep -q`：grep 命中即退出会让 zcat 收到 SIGPIPE，
# 在 pipefail 下整条管道被判失败，正常的备份也会误报。
gzip -t "$OUT"
copy_lines=$(gzip -dc "$OUT" | grep -c '^COPY ' || true)
if [ "${copy_lines:-0}" -eq 0 ]; then
  echo "备份校验失败，内容不完整: $OUT" >&2
  exit 1
fi

find "$DEST" -name 'dramarouter-*.sql.gz' -mtime +"$KEEP_DAYS" -delete
echo "备份完成: $OUT ($(du -h "$OUT" | cut -f1))，保留 ${KEEP_DAYS} 天"
