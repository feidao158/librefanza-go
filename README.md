# LibreFanza

Go 后端提供版本化 REST API、初始化向导与数据库持久化任务；Vue 3 前端使用 TypeScript、Tailwind CSS 和 daisyUI。支持 MySQL 8.0+、PostgreSQL 14+，Redis 仅作为可选缓存，不参与认证和任务队列。

## 本地开发

依赖 Go 1.27+、Node.js 24+ 和一个已创建的 MySQL 或 PostgreSQL 数据库。数据库用户必须拥有目标数据库的建表和修改权限。首次安装不会清空已有表；请使用专用空数据库。

```bash
cd frontend
npm ci
npm run build
cd ../backend
LIBREFANZA_INSECURE_COOKIES=true go run ./cmd/server
```

访问 `http://127.0.0.1:8080`，在向导中填写数据库连接、可选 Redis 和管理员账号。安装成功后，数据库配置保存在 `backend/data/config.json`，文件权限为 `0600`；备份时需要同时保存此目录和数据库。不要将配置文件加入版本控制。

首次安装没有额外的安装令牌，所有能访问服务的人都可能抢先初始化。初始化之前必须通过本机绑定、内网或防火墙限制访问。初始化完成后安装入口关闭；数据库故障不会重新开放安装。

仅在本机 HTTP 开发时设置 `LIBREFANZA_INSECURE_COOKIES=true`。生产环境应通过 HTTPS 反向代理访问，保持默认安全 Cookie。Web 前端使用 HTTP-only 刷新 Cookie；App 使用 `client: mobile` 获取刷新令牌并保存在系统安全存储中。

开发前端时可在另一个终端运行 `cd frontend && npm run dev`，Vite 将 `/api` 代理到本机 8080 端口。完成调试后停止两个进程。

## 容器运行

使用 `compose.yaml`，选择一个数据库配置；Redis 可通过额外 profile 启用。请在运行前通过环境或私有 `.env` 文件设置 `DB_PASSWORD`，可单独设置 `DB_ROOT_PASSWORD`。示例仅绑定本机端口，避免将未初始化的服务直接暴露到公网。

仅在本机通过 HTTP 试用时，可额外设置 `LIBREFANZA_INSECURE_COOKIES=true`，否则浏览器不会在 HTTP 连接上发送安全刷新 Cookie。生产环境应使用 HTTPS，并保持该选项关闭。

```bash
docker compose --profile postgres up -d --build
docker compose --profile mysql up -d --build
docker compose --profile postgres --profile redis up -d --build
docker compose down
```

以上三条 `up` 命令是互斥的部署示例，选择其中一条。容器内的数据库主机分别填写 `postgres` 或 `mysql`；启用 Redis 时地址填写 `redis:6379`。应用配置与数据库均使用 Docker 卷持久化。迁移数据库或恢复备份时，应同时保留应用数据卷，禁止删除该卷后直接重开安装向导。

直接运行时默认只监听 `127.0.0.1:8080`；容器内监听 `:8080`，Compose 将端口绑定到宿主机本地。默认工作目录由 `LIBREFANZA_DATA_DIR` 控制。也可设置 `LIBREFANZA_LISTEN`、`LIBREFANZA_FRONTEND_DIR`、`FANZA_API_ID`、`FANZA_AFFILIATE_ID`、`PROXY_URL`、`FC2_BASE_URL`、`MGSTAGE_SERIES_URL` 和 `BLACKHOLE_PATTERN`。管理员界面保存的来源配置优先于对应环境默认值。日志不打印数据库连接密码或来源 API 密钥。

默认不信任转发的客户端 IP；反向代理部署时，可通过 `LIBREFANZA_TRUSTED_PROXIES` 设置可信代理 IP 或 CIDR，多个值用逗号分隔。仅填写自己控制的代理地址，以便登录和初始化限速识别真实客户端；不应信任所有来源。

## 功能与接口

- 影片、演员浏览与筛选，来源版本、演员作品及单体作品查看。
- 未收录番号的异步检索按 Fanza、SOD、MGStage、FC2 顺序执行；JavLibrary 支持手动导入。
- 管理员可调整封面和来源优先级、重新解析来源、查看原始页面、配置采集与 Redis，并查看和重试任务。原始 HTML 以文本形式显示。
- 定时采集默认使用 `America/Los_Angeles` 时区，分别在 00:00、01:00、02:00 执行演员、Fanza 影片和 MGStage 采集；可以在管理界面修改时区、Cron 表达式及启用状态。
- 工作进程异常退出后，其他工作进程会回收约两分钟未更新的任务并重试；任务处理需要保持幂等，避免进程在写入后退出造成重复执行。
- 配置 SMTP 后提供密码重置；重置令牌一次性有效，更新密码会撤销旧会话。
- OpenAPI 文档：`GET /api/v1/openapi.yaml`。业务接口使用 `/api/v1` 前缀；列表响应包含 `data` 与 `meta`，错误包含 `error.code`。

## 测试

```bash
cd backend
go test ./...
go vet ./...
cd ../frontend
npm ci
npm test
npm run build
```

数据库集成测试需要专用、可写的 `librefanza_test` 数据库和测试用户，并通过环境变量启用。测试会创建和删除测试记录，不得指向生产数据库。

```bash
LIBREFANZA_TEST_DB_DRIVER=postgres LIBREFANZA_TEST_DB_PORT=5432 go test -p 1 ./... -count=1
LIBREFANZA_TEST_DB_DRIVER=mysql LIBREFANZA_TEST_DB_PORT=3306 go test -p 1 ./... -count=1
```

集成测试使用的测试数据库连接参数见测试源码；设置 `LIBREFANZA_TEST_REDIS_PORT` 可额外运行 Redis 集成用例。浏览器测试需要已启动的新服务和 Playwright Chromium：`cd frontend && npm run test:e2e`。如使用本机 Chrome，可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定其路径。

不提供旧系统数据或账号迁移。源代码历史可通过 Git 查看。
