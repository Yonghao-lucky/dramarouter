# 快速接入

本文介绍平台整体能力的接入流程与统一规范。所有请求共用下方实际网关地址；凭证、模型和资源 ID 请替换为自己的值。

## 基础配置

**网关地址：`https://realmdrama.com`**

请求体统一使用 JSON，请求头填写 `Content-Type: application/json`。调用地址由网关地址和接口路径组成，例如 `https://realmdrama.com/v1/chat/completions`。

| 接口分类 | 鉴权方式 | 请求头格式 | 获取入口 |
| --- | --- | --- | --- |
| 文本模型、图像生成、视频生成 | 模型 API Key | `Authorization: Bearer YOUR_API_KEY` | 控制台「令牌」页面创建 |
| 素材库 Action API | AK/SK V4 签名 | `Authorization: HMAC-SHA256 Credential=YOUR_AK/...` | 控制台「素材库 → API 接入」创建 |
| 已签名的视频/图片下载地址 | 免 Header 鉴权 | 普通 HTTP GET，不携带 Authorization | 任务生成成功后从响应中获取 |

## 核心业务调用流程

### 1. 文本模型
通过兼容 OpenAI 的标准协议接入文本对话与补全，支持单轮/多轮会话以及流式（SSE）增量输出：
- 接口路径：`POST /v1/chat/completions`
- 详细参数说明、同步/流式调用代码示例见「文本模型」。

### 2. 图像生成
提供兼容 OpenAI 的文生图、图生图/编辑，以及火山方舟原生生图接口：
- 接口路径：`POST /v1/images/generations`（文生图）、`POST /v1/images/edits`（图生图）、`POST /api/v3/images/generations`（方舟原生）
- 支持 URL 直链、Base64 编码与 SSE 流式返回，详见「图像生成」。

### 3. 素材库与视频生成
视频生成支持公网图片直链、Base64 直传以及素材库资产引用（`asset://YOUR_ASSET_ID`）：
1. **素材上传（可选）**：通过素材库 API 导入图片，取得素材 ID；等待状态变为 `Active` 后即可在视频生成中复用。
2. **提交任务**：强烈推荐调用火山方舟官方原生接口 `POST /api/v3/contents/generations/tasks`（★ 首选推荐，完整支持官方特性与 TOS 极速直链）；存量系统亦支持兼容接口 `POST /v1/video/generations`（不推荐）。
3. **获取结果**：通过任务查询接口获取状态，或配置 Webhook 自动接收完成通知。
4. **下载视频**：任务成功后读取 `video_url`，直接下载并转存。
- 完整参数规范、查询轮询与下载示例见「视频生成」与「素材库 API」。

### 4. 事件回调通知 (Webhook)
当耗时较长的异步任务完成或素材审核状态变更时，平台会主动向您配置的公网 HTTPS 端点推送通知：
- **媒体任务回调**：订阅视频等任务状态流转，生成完成或失败时第一时间推送，见「媒体任务 Webhook」。
- **素材审核回调**：订阅素材异步审核结果，入库激活或被拒时即时推送，见「素材库 Webhook」。
