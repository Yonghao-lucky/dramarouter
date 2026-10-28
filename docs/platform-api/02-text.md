# 文本模型

平台提供兼容 OpenAI 标准协议的文本对话与补全接口 `POST /v1/chat/completions`。支持多轮对话、角色设定（system / user / assistant）以及流式（SSE）增量输出。

## 接口说明与鉴权

- **接口路径**：`POST /v1/chat/completions`
- **请求头**：
  - `Content-Type: application/json`
  - `Authorization: Bearer YOUR_API_KEY`（在控制台「令牌」页面创建并获取）

## 方式一 · 同步调用 (stream = false)

发送请求后，服务端生成完毕一次性返回完整结果。

### 请求示例

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/chat/completions' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_TEXT_MODEL_ID",
  "messages": [
    {
      "role": "system",
      "content": "你是一个专业、严谨的智能助手。"
    },
    {
      "role": "user",
      "content": "请用一句话介绍你的能力与特点。"
    }
  ],
  "temperature": 0.7,
  "stream": false
}'
```

### 响应示例

```json
{
  "id": "chatcmpl-7x89abc123456",
  "object": "chat.completion",
  "created": 1791264000,
  "model": "YOUR_TEXT_MODEL_ID",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "我是一个通用人工智能助手，能够协助您完成文本创作、代码编写、数据分析与知识问答等多种任务。"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 28,
    "completion_tokens": 36,
    "total_tokens": 64
  }
}
```

客户端读取 `choices[0].message.content` 即可获得模型的完整回复。

## 方式二 · 流式调用 (stream = true)

将 `stream` 设为 `true` 后，服务端按 **Server-Sent Events (SSE)** 协议实时分块返回生成内容，适用于打字机效果、实时对话等低延迟交互场景。

### 请求示例

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/chat/completions' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_TEXT_MODEL_ID",
  "messages": [
    {
      "role": "user",
      "content": "写一首关于秋天稻田的四句小诗。"
    }
  ],
  "stream": true
}'
```

### 响应数据流格式

服务端以 `text/event-stream` 格式持续推送事件，每行以 `data: ` 开头：

```text
data: {"id":"chatcmpl-7x89","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"稻"}}]}

data: {"id":"chatcmpl-7x89","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"浪"}}]}

data: {"id":"chatcmpl-7x89","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"金"}}]}

data: {"id":"chatcmpl-7x89","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"黄..."}}]}

data: [DONE]
```

客户端逐块提取 `choices[0].delta.content` 拼接显示，接收到 `data: [DONE]` 时表示本次流式生成结束。

## 请求参数说明

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | - | 模型名称，需与控制台开通的模型 ID 一致 |
| `messages` | array | 是 | - | 结构化对话上下文列表，见下方消息对象说明 |
| `temperature` | number | 否 | 0.7 | 采样温度（0 ~ 2），值越大回复越富有创意，值越小越严谨集中 |
| `top_p` | number | 否 | 1.0 | 核采样阈值，建议与 `temperature` 仅调整其中一个 |
| `max_tokens` | integer | 否 | - | 单次生成最大 Token 限制，超出后将被截断 |
| `stream` | boolean | 否 | false | 是否以 SSE 增量流式输出回复 |
| `presence_penalty` | number | 否 | 0 | 存在惩罚（-2.0 ~ 2.0），正值鼓励模型引入新话题 |
| `frequency_penalty`| number | 否 | 0 | 频率惩罚（-2.0 ~ 2.0），正值降低模型重复相同词汇的概率 |

### messages 消息对象

`messages` 数组按对话顺序传入，每个元素包含：

- `role`：角色标识，支持 `system`（系统人设设定）、`user`（用户输入）、`assistant`（历史模型回复）。
- `content`：该轮对话的文本内容。

多轮对话时，将历史 `user` 与 `assistant` 消息按时间先后顺序全部放入 `messages` 中提交即可。

## 常见错误与排查

| 状态码 | 错误码 | 常见原因与解决建议 |
| --- | --- | --- |
| 401 | `Unauthorized` | 请求头未携带 Authorization 或 API Key 无效；请在控制台「令牌」重新复制有效密钥 |
| 400 | `InvalidRequest` | JSON 格式不合法，或缺少必填的 `model` / `messages` 参数 |
| 404 | `ModelNotFound` | 传入的 `model` 不存在，或当前 API Key 未关联该模型的调用权限 |
| 429 | `RateLimit / InsufficientQuota` | 账号余额不足或并发超出频率限制；请在控制台核查额度并进行充值 |
| 500 | `InternalServerError` | 上游模型供应商服务波动或网络超时；建议配置重试机制或稍后重试 |
