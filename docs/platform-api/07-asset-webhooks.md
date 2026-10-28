# 素材库 Webhook

素材库 Webhook 用于接收图片等文件上传导入后的异步审核结果通知。导入成功不等于素材立即可用，必须等待状态变为 `Active` 后，方可在视频生成任务中通过 `asset://YOUR_ASSET_ID` 引用；若审核被拒绝变为 `Failed`，需立即拦截新的任务引用。

## HTTP 协议与公共请求头

平台主动向您配置的公网 HTTPS 地址发送 POST 请求：

```http
POST /webhooks/assets HTTP/1.1
Content-Type: application/json
User-Agent: New-API/1.0
webhook-id: wh_sample_asset_delivery
webhook-timestamp: 1791264000
```

| 请求头 | 说明 |
| --- | --- |
| `Content-Type` | 固定为 `application/json` |
| `User-Agent` | 固定为 `New-API/1.0` |
| `webhook-id` | 稳定投递 ID（`wh_` 前缀）；同一端点重试时保持不变，接收端用于**幂等去重** |
| `webhook-timestamp` | 本次请求发送时的 Unix 秒时间戳；重试时可能递增 |

平台不发送签名密钥，不跟随 HTTP 重定向。若需核验素材最新状态，可在关键动作前使用素材 AK/SK 调用 GetAsset 接口核验。

## 事件信封与回调示例

事件在订阅生效后产生，不回放历史素材。平台发送的 JSON 包含顶层信封：

```json
{
  "id": "evt_sample_asset_event",
  "object": "event",
  "type": "asset.failed",
  "created_at": 1791264000,
  "owner_user_id": 7,
  "data": {
    "id": "asset_sample_456",
    "group_id": "group_sample_789",
    "status": "Failed",
    "failure_reason": "policy_rejected",
    "updated_at": 1791264000,
    "state_version": 2
  }
}
```

## 业务字段说明 (data)

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 平台公开素材 ID，可作为 `asset://YOUR_ASSET_ID` 提交视频生成任务 |
| `group_id` | string? | 所属素材组公开 ID；存在素材组时提供 |
| `status` | string | 素材状态：`Active`（可用）或 `Failed`（审核拒绝） |
| `failure_reason` | string? | 审核失败原因码，仅在 `Failed` 状态且存在明确原因时提供 |
| `updated_at` | integer?| 素材状态更新时间，Unix 秒时间戳 |
| `state_version` | integer | 素材状态版本号：首次可用为 `1`，明确拒绝为 `2` |

## 状态与失败原因枚举

| 枚举值 | 类别 | 含义与说明 |
| --- | --- | --- |
| `asset.active` / `Active` | 事件 / 状态 | 首次确认素材可用（`state_version = 1`） |
| `asset.failed` / `Failed` | 事件 / 状态 | 素材被上游明确拒绝（`state_version = 2`），同一素材的终态 |
| `policy_rejected` | 失败原因 | 内容被上游策略规则拒绝 |
| `sensitive_content` | 失败原因 | 触发敏感内容风控规则 |
| `real_person` | 失败原因 | 包含真实人物相关风控规则 |

## 可靠接收最佳实践

1. **幂等去重**：以请求头 `webhook-id` 作为唯一键进行去重，避免重复入库。
2. **快速响应**：接收端持久化事件到本地队列后，应立刻返回 HTTP `200` 或 `204`。
3. **版本比对防乱序**：按素材 ID 原子比对 `state_version`，**只有新事件的 state_version 严格大于本地已记录版本时才更新**。若素材先可用后被复审改判拒绝，版本为 `2 > 1`，将正确更新为失败并阻止新任务引用；反之迟到的可用事件（版本 1）不会覆盖已存在的失败状态。
4. **失败处理原则**：素材状态变为 `Failed` 时，应立即在业务数据库打标并拦截后续视频生成引用，重新上传文件会生成新的素材 ID。

## 重试机制

- 平台单次请求超时约为 **10 秒**。
- 若接收端返回非 `2xx` 状态码、出现网络错误或响应超时，平台将自动采用**指数退避算法**重试。
- 重试最长保留 **72 小时**，单次重试间隔上限为 12 小时。重试期间 `webhook-id` 与消息体保持不变。

## 端点管理 API (可选)

如需脚本自动化管理，可使用平台账号个人凭证（请求头 `Authorization: Bearer YOUR_ACCESS_TOKEN`）调用以下 REST 接口：

| 操作 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 获取端点列表 | GET | `/api/webhook-endpoints` | 列出当前账号已配置的所有有效 Webhook 端点 |
| 新建端点 | POST | `/api/webhook-endpoints` | 注册新的端点，`event_types` 传入 `["asset.active", "asset.failed"]` |
| 修改端点 | PUT | `/api/webhook-endpoints/{id}` | 更新端点名称、URL 或订阅的事件类型 |
| 删除端点 | DELETE | `/api/webhook-endpoints/{id}` | 停用并移除指定端点 |
| 连通性测试 | POST | `/api/webhook-endpoints/{id}/test` | 立即向端点发送测试事件并返回 HTTP 状态与耗时 |

创建端点示例：

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/api/webhook-endpoints' \
  --header 'Authorization: Bearer YOUR_ACCESS_TOKEN' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "name": "素材审核通知服务",
  "url": "https://api.yourdomain.com/webhooks/assets",
  "event_types": [
    "asset.active",
    "asset.failed"
  ]
}'
```
