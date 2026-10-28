# 媒体任务 Webhook

媒体任务 Webhook 用于异步接收视频生成、Suno 音乐等主任务模型的状态变更通知。当任务在服务端产生状态流转（提交、排队、生成中、完成成功、失败或取消）时，平台会主动将最新状态与结果快照推送到外部接收端。

## HTTP 协议与公共请求头

平台主动向您配置的公网 HTTPS 地址发送 POST 请求：

```http
POST /webhooks/media HTTP/1.1
Content-Type: application/json
User-Agent: New-API/1.0
webhook-id: wh_sample_task_delivery
webhook-timestamp: 1791264000
```

| 请求头 | 说明 |
| --- | --- |
| `Content-Type` | 固定为 `application/json` |
| `User-Agent` | 固定为 `New-API/1.0` |
| `webhook-id` | 稳定投递 ID（`wh_` 前缀）；同一端点重试时保持不变，接收端用于**幂等去重** |
| `webhook-timestamp` | 本次请求发送时的 Unix 秒时间戳；重试时可能递增 |

平台不发送签名密钥，不跟随 HTTP 重定向。若需核验关键业务数据真实性，请调用对应的任务查询接口。

## 事件信封与回调示例

事件在订阅生效后产生，不回放历史任务。平台发送的 JSON 包含顶层信封：

```json
{
  "id": "evt_sample_task_event",
  "object": "event",
  "type": "task.status_changed",
  "created_at": 1791264000,
  "owner_user_id": 7,
  "data": {
    "task_id": "task_20261008_abc123",
    "platform": "54",
    "status": "SUCCESS",
    "state_version": 3,
    "progress": "100%",
    "response": {
      "status": "succeeded",
      "content": {
        "video_url": "https://realmdrama.com/v1/download/video/sample.mp4"
      }
    },
    "response_sha256": "3fa85f647f3942b036923e"
  }
}
```

## 业务字段说明 (data)

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 客户端查询任务的公开任务 ID |
| `platform` | string | 任务平台标识，视频渠道通常为数字字符串（如 DoubaoVideo 为 `54`），音频为 `suno` |
| `status` | string | 任务统一状态，完整枚举见下表 |
| `state_version` | integer | 任务状态版本号，单调递增，接收端用于乱序防护 |
| `progress` | string | 任务处理进度文本，如 `"100%"`；仅更新进度不触发新事件 |
| `fail_reason` | string? | 任务失败原因说明，仅在失败时提供 |
| `response` | object? | 任务状态变化时的不可变响应快照；若成功可读取 `content.video_url` |
| `response_sha256` | string? | 原始响应快照摘要，用于排查关联，并非回调签名 |
| `response_truncated`| boolean?| `true` 表示响应中超长 Base64 或超大文本已替换为省略提示 |
| `response_omitted`  | boolean?| `true` 表示原文超过 8 MiB 或处理后仍超过 64 KiB 已省略，需调用查询接口获取 |

## 任务状态枚举

| 状态 | 含义与说明 |
| --- | --- |
| `NOT_START` | 任务已创建，等待提交上游 |
| `SUBMITTED` | 已向模型服务建立提交请求 |
| `QUEUED` | 任务在上游队列排队中 |
| `IN_PROGRESS` | 模型正在生成或处理中 |
| `SUCCESS` | 任务成功完成，可从 `response` 读取视频或生成内容 |
| `FAILURE` | 任务生成失败、超时或确认取消，可结合 `fail_reason` 诊断 |
| `UNKNOWN` | 状态未知，应调用任务查询接口核验 |

## 可靠接收最佳实践

1. **幂等去重**：以请求头 `webhook-id` 作为唯一键进行去重，避免网络抖动或平台重试引发重复入库。
2. **快速响应**：接收端将事件持久化到自己的本地队列后，应立刻返回 HTTP `200` 或 `204`；文件下载、转存等耗时业务逻辑必须异步处理。
3. **版本比对防乱序**：由于网络可能存在乱序延迟，接收端必须比对 `data.state_version`，**只有新事件的 state_version 严格大于本地已记录的版本时才更新**，切勿让迟到的排队或中间事件覆盖最终的成功/失败状态。

## 重试机制

- 平台单次请求超时约为 **10 秒**。
- 若接收端返回非 `2xx` 状态码、出现网络错误或响应超时，平台将自动采用**指数退避算法**重试。
- 重试最长保留 **72 小时**，单次重试间隔上限为 12 小时。重试期间 `webhook-id` 与消息体保持不变。

## 端点管理 API (可选)

如需脚本自动化管理，可使用平台账号个人凭证（请求头 `Authorization: Bearer YOUR_ACCESS_TOKEN`）调用以下 REST 接口：

| 操作 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 获取端点列表 | GET | `/api/webhook-endpoints` | 列出当前账号已配置的所有有效 Webhook 端点 |
| 新建端点 | POST | `/api/webhook-endpoints` | 注册新的端点，`event_types` 传入 `["task.status_changed"]` |
| 修改端点 | PUT | `/api/webhook-endpoints/{id}` | 更新端点名称、URL 或订阅的事件类型 |
| 删除端点 | DELETE | `/api/webhook-endpoints/{id}` | 停用并移除指定端点 |
| 连通性测试 | POST | `/api/webhook-endpoints/{id}/test` | 立即向端点发送测试事件并返回 HTTP 状态与耗时 |

创建端点示例：

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/api/webhook-endpoints' \
  --header 'Authorization: Bearer YOUR_ACCESS_TOKEN' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "name": "媒体任务通知服务",
  "url": "https://api.yourdomain.com/webhooks/media",
  "event_types": [
    "task.status_changed"
  ]
}'
```
