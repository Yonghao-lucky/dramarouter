# 图像生成

平台提供兼容 OpenAI 标准协议的生图接口 `POST /v1/images/generations`（文生图）、`POST /v1/images/edits`（图生图 / 图像编辑），以及火山引擎方舟原生生图接口 `POST /api/v3/images/generations`。支持图片 URL 直链、Base64 编码、多图生成与 SSE 流式增量输出。

## 接口说明与鉴权

- **认证方式**：模型 API Key
- **请求头**：
  - `Authorization: Bearer YOUR_API_KEY`（在控制台「令牌」页面创建并获取）
  - `Content-Type: application/json`（文件上传时使用 `multipart/form-data`）

| 接口用途 | 请求方式 | 接口路径 | 支持特性 |
| --- | --- | --- | --- |
| 文生图 (OpenAI 兼容) | POST | `/v1/images/generations` | 提示词生成、多分辨率、Base64/URL、SSE 流式 |
| 图生图 / 编辑 (OpenAI 兼容) | POST | `/v1/images/edits` | 参考图垫图、局部重绘、蒙版 Mask、多图生成 |
| 火山方舟原生生图 | POST | `/api/v3/images/generations` | 火山引擎方舟原生格式透传、定制参数直通 |

## 场景一 · 文生图 (POST /v1/images/generations)

### 1. 同步生成返回图片直链 (默认)

服务端生成完毕后，返回带有效期的公网可直接下载的图片 URL 直链。

#### 请求示例

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/images/generations' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_IMAGE_MODEL_ID",
  "prompt": "清晨薄雾笼罩下的金色稻田，日光穿透云层洒向大地，电影质感，超清写实细节",
  "n": 1,
  "size": "1024x1024",
  "quality": "standard",
  "response_format": "url"
}'
```

#### 响应示例

```json
{
  "created": 1791264000,
  "data": [
    {
      "url": "https://realmdrama.com/v1/download/image/sample_01.png",
      "revised_prompt": "清晨薄雾笼罩下的金色稻田，日光穿透云层洒向大地，电影质感，超清写实细节"
    }
  ]
}
```

客户端直接读取 `data[0].url` 即可下载或展示图片。

### 2. 返回 Base64 编码图片

将 `response_format` 设为 `b64_json`，适合内网隔离环境或无需外链存储的场景：

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/images/generations' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_IMAGE_MODEL_ID",
  "prompt": "秋季丰收的稻穗特写，微距摄影，金黄色颗粒饱满",
  "response_format": "b64_json"
}'
```

响应中的 `data[0].b64_json` 即为图片的 Base64 字符串（不带 `data:image/png;base64,` 前缀），解码后即可直接存为二进制文件。

### 3. SSE 流式生成 (stream = true)

将 `stream` 设为 `true` 后，服务端按 **Server-Sent Events (SSE)** 协议实时推流：

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/images/generations' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_IMAGE_MODEL_ID",
  "prompt": "现代化农业大棚内的智能灌溉系统，俯拍全景，4K画质",
  "stream": true
}'
```

#### 响应数据流格式

```text
event: image_generation.completed
data: {"type":"image_generation.completed","created_at":1791264000,"url":"https://realmdrama.com/v1/download/image/sample_02.png","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}

data: [DONE]
```

## 场景二 · 图生图与编辑 (POST /v1/images/edits)

用于垫图生成、图像修改或局部重绘。支持 JSON 与 `multipart/form-data` 两种方式。

### JSON 调用（URL / Base64 输入）

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/images/edits' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_IMAGE_MODEL_ID",
  "image": "https://files.example.com/assets/origin-field.jpg",
  "prompt": "将背景由白天改为夕阳西下的落日余晖，整体暖橙色调",
  "size": "1024x1024"
}'
```

### Multipart 上传本地文件

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/v1/images/edits' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --form 'model="YOUR_IMAGE_MODEL_ID"' \
  --form 'image=@"origin-photo.png"' \
  --form 'prompt="将画风转换为水彩插画手绘风格"' \
  --form 'size="1024x1024"'
```

## 场景三 · 火山方舟原生生图 (POST /api/v3/images/generations)

当需要直接使用火山引擎方舟原生数据结构与高级参数时，可直接请求原生路由：

```bash
curl --fail-with-body --request POST 'https://realmdrama.com/api/v3/images/generations' \
  --header 'Authorization: Bearer YOUR_API_KEY' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "model": "YOUR_IMAGE_MODEL_ID",
  "prompt": "蓝天白云下金黄色的麦田，广角镜头，画质逼真细腻",
  "width": 1024,
  "height": 1024
}'
```

平台将原样透传至火山引擎上游通道，并自动完成用量扣费与响应规范化。

## 请求参数说明 (POST /v1/images/generations)

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | - | 生图模型名称，需与控制台开通的模型 ID 一致 |
| `prompt` | string | 是 | - | 画面描述提示词，描述画面主体、背景、光影与艺术风格 |
| `n` | integer | 否 | 1 | 生成图片张数，单次请求最大支持 128 张（视具体模型并发上限而定） |
| `size` | string | 否 | `1024x1024` | 图片分辨率尺寸，如 `1024x1024`、`1024x1792`、`1792x1024`、`512x512`（**注意：中间必须为小写字母 `x`，不能使用乘号 `×`**） |
| `quality` | string | 否 | `standard` | 图像质量，可选 `standard`（标准画质）或 `hd`（高清画质） |
| `response_format` | string | 否 | `url` | 输出数据格式：`url`（HTTP 图片下载直链）或 `b64_json`（Base64 编码） |
| `stream` | boolean | 否 | false | 是否以 SSE 增量事件流输出回复 |
| `watermark` | boolean | 否 | false | 是否在生成图片中保留或添加平台水印（需模型支持） |
| `style` | string | 否 | - | 画面风格偏好，部分模型支持 `vivid`（鲜明高饱和）或 `natural`（自然写实） |

## 常见错误与排查

| 状态码 | 错误码 | 常见原因与解决建议 |
| --- | --- | --- |
| 401 | `Unauthorized` | 未提供有效的 API Key，请检查 `Authorization: Bearer` 请求头并在控制台核验令牌有效性 |
| 400 | `invalid_request` | 缺少必填参数 `model` 或 `prompt`；或 `size` 参数中误用了乘号 `×` 而非字母 `x` |
| 404 | `model_not_found` | 请求的模型不存在，或当前 API Key 绑定的令牌未勾选该模型的调用权限 |
| 429 | `insufficient_quota` | 账户额度不足或触发了频率限制，请登录控制台充值或调整并发调用节奏 |
| 500 | `upstream_request_failed` | 上游模型供应商生成超时或出现网络故障，建议在客户端增加指数退避重试逻辑 |
