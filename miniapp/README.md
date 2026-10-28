# 微信小程序

这是当前仓库中的独立 Taro 4 + React 18 小程序应用。它复用现有 Go API 和业务规则，但不导入 `web/src` 的 DOM 组件或 React 19 运行时。

长期计划、任务编号、依赖关系与验收证据见 [`docs/design/WECHAT_MINIAPP_PROJECT_PLAN.zh_CN.md`](../docs/design/WECHAT_MINIAPP_PROJECT_PLAN.zh_CN.md)。

## 本地启动

要求：Bun、微信开发者工具，以及正在运行的本仓库 Go 后端。

```bash
cd miniapp
bun install
make dev-local
```

然后在微信开发者工具中导入 `miniapp/`。开发骨架使用 `touristappid`，需要登录、真机、合法域名或支付能力时，在本机的 `project.private.config.json` 中配置真实 AppID；该文件不会提交。

`MINIAPP_API_BASE_URL` 会在构建时写入客户端，只能放公开的 API 基地址，不能放任何 Secret。`Makefile` 固定了本地 `http://127.0.0.1:3000` 和远端 `https://realmdrama.com` 两个地址：`make local` / `make remote` 分别生成对应的 `dist/`，`make dev-local` / `make dev-remote` 用于持续监听构建。正式环境必须使用已在微信公众平台配置的 HTTPS 合法域名。
每次重新构建都要显式选择目标环境；未传入 `MINIAPP_API_BASE_URL` 时虽然可以编译，但小程序不会连接后端。录音相关隐私声明需在微信小程序管理后台完成，不要在 `app.json` 中添加开发者工具不支持的 `permission.scope.record` 字段。

## 真机与环境切换

`127.0.0.1` 在真机上指手机本身，不是运行 Go 后端的电脑。真机预览和首次上传前，请用固定的远端 HTTPS 地址重新构建：

```bash
cd miniapp
make remote
```

需要在远端地址下持续监听构建时可运行 `make dev-remote`。打开微信开发者工具的 `miniapp/` 项目后，使用真实 AppID 预览到手机；后端须已在公网域名部署，并配置同一小程序 AppID/Secret。`MINIAPP_API_BASE_URL` 是编译时配置，不会因选择“开发版、体验版、正式版”自动变化；上传前应运行 `make remote`，并重新预览或上传。`make local` 会覆盖同一个 `dist/`，不能将其产物上传到微信。

在微信公众平台为该小程序配置 HTTPS `request` 和录音上传所需的 `uploadFile` 合法域名；媒体文件经过 `downloadFile` 获取时，还需配置实际下载目标的 `downloadFile` 合法域名。当前对话流式响应使用 HTTPS `request`，并非 WebSocket。开发者工具或真机调试模式的域名校验开关只能用于临时排查，体验版/正式版验收必须关闭该开关，使用有效证书和合法域名。

## 验证

```bash
bun run typecheck
bun run lint
bun run test
make remote
```

构建产物位于 `dist/`。首页调用公开的 `/api/status` 验证平台连接；当前已覆盖微信登录与账号连接、首页概览、模型目录、API Key、流式对话、用量/任务/账单、钱包与充值码、订阅余额购买、账户语言、登录会话、签到推广和隐私注销。微信支付 V3 必须等待运营方提供主体资质、商户配置和合规结论，不会使用 WebView 或客户端入账方式绕过。

对话的图片附件、语音输入和回答朗读默认关闭。管理员在浏览器端“系统设置 → 模型 → 小程序媒体能力”中分别选定可用的对话模型，再配置支持 `/v1/audio/transcriptions` 的转写模型、支持 `/v1/audio/speech` 的合成模型及音色。录音先转成可编辑文字；朗读按需生成，不要求问答模型自身支持音频。图片只会发往已确认支持图片输入的问答模型。三种请求均使用当前用户分组、授权和平台计费；上游渠道模型名及接口需由运营方实际验证。录音还需要用户授予麦克风权限，并在小程序平台完成相应隐私声明。图片最大 1 MB，录音文件最大 10 MB；图片随本机历史记录保存，历史记录不会跨设备同步。

后端管理员需要通过现有系统设置能力配置以下键，并在最后启用登录：

```text
wechat_miniapp.app_id
wechat_miniapp.app_secret
wechat_miniapp.request_timeout_seconds
wechat_miniapp.enabled
```

也可以在仓库根目录 `.env` 或部署环境中设置以下变量。显式环境变量优先于数据库系统设置；系统环境变量又优先于 `.env` 中的同名值：

```dotenv
WECHAT_MINIAPP_APP_ID=wx_your_app_id
WECHAT_MINIAPP_APP_SECRET=replace_with_your_app_secret
WECHAT_MINIAPP_ENABLED=true
WECHAT_MINIAPP_REQUEST_TIMEOUT_SECONDS=5
```

AppSecret 只保存在服务端设置中。公开状态接口只返回 `wechat_miniapp_login` 就绪布尔值；小程序不会收到 AppSecret、OpenID 或微信 `session_key`。

已有平台账号可在“我的 → 使用已有账号登录”中输入用户名、邮箱或系统已分配手机号及密码；如该账号启用两步验证，还需输入动态码或备用码。此路径使用 `/api/mini/auth/password`，不调用微信手机号授权或 `wx.login`，但须开启平台密码登录。若微信手机号授权不可用，小程序会直接展示该表单；这不会自动绑定微信身份，也不会仅凭手机号登录。首次小程序新账号注册仍需微信验证手机号。

## 目录

```text
config/             Taro 构建配置
src/api/            API 地址、请求封装和接口适配
src/components/     小程序轻量共享组件
src/hooks/          页面级 Taro/React Hook
src/i18n/           七语言资源与语言检测
src/auth/           版本化本地会话、微信登录和绑定/注册适配
src/account/        账户偏好解析与高风险确认规则
src/pages/          五个 Tab 页面及功能详情页面
```

## 边界

- Web 与小程序依赖、锁文件和构建完全隔离。
- 跨端共享优先使用后端 API 契约；未来确需共享 TypeScript 时，只能放无运行时依赖的纯类型。
- 身份、计费、权限、限流和支付最终状态都以后端为准。

## 构建告警检查

CI 执行 `bun run lint --deny-warnings` 和 `bun run build:weapp:check`，任何 lint 或构建告警都会失败。小程序按同步资源的实际约束设置单资源 512 KiB、入口 2 MiB 的构建预算，超出预算直接报错；不使用 Webpack 默认的浏览器 244 KiB 提示，也不关闭其他告警。微信开发者工具中的真实上传包体积限制仍需验收。

浏览器微信登录的独立网站应用开通及环境变量配置见 [联合登录说明](../docs/WECHAT_WEB_LOGIN.zh_CN.md)。
