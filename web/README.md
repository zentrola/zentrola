# zentrola 管理界面

位于现有仓库的 `web/`，使用 Vue 3、TypeScript、Vue Router、vue-i18n 和 Vite。依赖版本与 lockfile 一起固定；框架用法参考 [Vue 官方指南](https://vuejs.org/guide/quick-start.html)，开发代理参考 [Vite 服务配置](https://vite.dev/config/server-options.html)。

## 开发

日常开发测试统一使用 Vite 开发服务（`npm run dev`，端口 `5173`），前端修改通过热更新生效。后端修改后重新编译并重启本地 Go 服务（端口 `8080`）；Docker 镜像构建和容器更新仅用于明确需要的部署验证，不作为日常开发步骤。

使用 Node.js 24 LTS。先在仓库根目录启动已有 Go 服务，再在另一终端执行：

```powershell
cd D:\Workspace\Private\zentrola\web
npm.cmd ci
npm.cmd run dev
```

打开 `http://127.0.0.1:5173`。浏览器只访问当前前端域名，Vite 将 `/api` 代理至 `http://127.0.0.1:8080`，无需为本地开发放开跨域。后端端口不同可在启动 Vite 前设置：

```powershell
$env:ZENTROLA_API_TARGET = 'http://127.0.0.1:8081'
npm.cmd run dev
```

该变量仅用于开发服务器，不打包到浏览器。前端不读取后端 `.env`，不配置上游 API Key 或管理员密码。不要将秘密放入 `VITE_*` 变量。

新环境首次打开页面时自行设置管理员账号、密码和确认密码，完成后登录；已有管理员时直接显示登录页，不设默认账号密码，升级不修改已有账号。登录后将 JWT 和后端到期时间保存至同源 localStorage（`zentrola.admin.session`），刷新或重新打开页面时通过 `/me` 校验并恢复身份，不延长原有 8 小时有效期。退出、过期或管理 API 返回 401 时清除存储、身份和在途请求，并同步同源标签页。恢复时的临时网络错误保留凭证并提供重试；浏览器禁用存储时降级为当前页面会话。密码、上游凭证和业务数据不写入浏览器存储。详见 [首次管理员初始化](../docs/首次管理员初始化.md)。

运行真实本地服务验收脚本 `tests/live.mjs` 时，需通过进程环境变量 `ZENTROLA_TEST_ADMIN_USERNAME` 和 `ZENTROLA_TEST_ADMIN_PASSWORD` 显式提供已有管理员的账号密码；脚本不读取后端 `.env`，也不使用默认账号。

## 构建与运行

```powershell
# web 目录
npm.cmd run build
# 回到仓库根目录，从根目录启动 Go 服务
cd ..
go run ./cmd/server
```

浏览器访问后端根地址（默认 `http://127.0.0.1:8080/`）即可打开界面。Go 提供 `web/dist/index.html`、`/assets/*` 和 favicon；构建产物需随本地二进制一起保留，运行工作目录为仓库根目录。未构建时页面返回 404，API 仍可独立使用。页面使用 hash 路由，刷新 `/#/resources` 等地址不需要额外重写规则。

`docker compose up --build -d` 会在 Node 构建阶段生成页面并复制进 Go 镜像，仍只有 `app + postgres` 两个服务。生产镜像不包含 Node、源码、node_modules 或后端 `.env`。静态资源与 API 同源，页面启用 CSP、禁止嵌入和 MIME 嗅探。

## 目录

```text
src/
  api.ts             API 响应、认证、超时与错误码映射
  types.ts           响应类型，业务 ID 始终为字符串
  i18n.ts            中文词条，保留多语言扩展结构
  composables.ts     游标分页、操作状态、格式化与校验
  components/        弹窗、状态、图标、页面标题及分页
  pages/             成员、分组、模型、资源、用量及日志
  App.vue            登录、导航和管理员布局
  style.css          全局视觉规范和响应式样式
tests/
  admin.spec.ts      模拟 API 的完整浏览器业务测试
  live.mjs           显式执行的真实本地后端验收
  setup.spec.ts      首次初始化与并发抢先完成的浏览器测试
  setup-live.mjs     显式执行的空库首次设置验收
```

页面不提供演示数据或假统计。列表按游标加载更多，搜索框明确只搜索已加载记录；分组候选项和用量筛选选项会遍历全部分页。业务错误按 `code` 显示中文说明并保留 requestId，不依赖后端英文 message。

点击右上角账号打开菜单，可选择“修改密码”或“注销登录”。修改对话框需要填写当前密码、新密码及确认新密码。新密码沿用初始化时的 12–72 UTF-8 字节规则；修改成功后清除本地会话，并使该账号所有设备的旧登录凭证失效。移动端保留头像菜单入口。

## 检查

```powershell
npm.cmd run build
# 首次安装浏览器；保留本机其他版本的 Playwright 浏览器。
$env:PLAYWRIGHT_SKIP_BROWSER_GC = '1'
npx.cmd playwright install chromium
npm.cmd test
```

默认浏览器测试自动启动 Vite，并拦截管理 API，仅使用隔离的内存数据，不访问真实后端。截图只包含模拟数据，放在被忽略的 `test-results/visual`；不启用 trace、视频或失败截图，避免后续真实验收录下凭证。

真实本地验收脚本需要独立测试实例（默认 `http://127.0.0.1:8081`）、已有 `DeepSeek 开发资源` 及 Flash 模型。通过 `ZENTROLA_TEST_ADMIN_USERNAME / ZENTROLA_TEST_ADMIN_PASSWORD` 提供已设置的管理员凭证；为兼容旧开发库，脚本也可从根目录 `.env` 读取遗留的初始账号配置，这仅是测试脚本的兼容行为。它会创建临时成员和分组、授予模型、签发 15 分钟 Key，然后撤销 Key、移除关联并停用成员，保留业务及审计历史；连接检查只查询模型列表，不发起模型推理或覆盖已有凭证。

```powershell
# 仅在需要真实验收时显式执行
$env:ZENTROLA_WEB_URL = 'http://127.0.0.1:8081'
node tests/live.mjs
```

脱敏报告写入根目录 `.cache/web-live-results.json`；截图在 `.cache/web-visual`，不拍摄凭证录入和 Key 展示状态。普通 `npm test` 不执行此脚本。
