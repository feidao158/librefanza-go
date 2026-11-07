# LibreFanza

Go 后端提供版本化 REST API、初始化向导与数据库持久化任务；Vue 3 前端使用 TypeScript、Tailwind CSS 和 daisyUI。支持 MySQL 8.0+、PostgreSQL 14+，Redis 仅作为可选缓存，不参与认证和任务队列。

## 本地开发

依赖 Go 1.27+、Node.js 24+ 和一个已创建的 MySQL 或 PostgreSQL 数据库。数据库用户必须拥有目标数据库的建表和修改权限。首次安装不会清空已有表；请使用专用空数据库。

```bash
cd frontend
npm ci
npm run build
cd ../backend
go run ./cmd/server
```

访问 `http://127.0.0.1:8080`，在向导中填写数据库连接、可选 Redis 和管理员账号。安装成功后，数据库配置保存在 `backend/data/config.json`，文件权限为 `0600`；备份时需要同时保存此目录和数据库。不要将配置文件加入版本控制。

首次安装没有额外的安装令牌，所有能访问服务的人都可能抢先初始化。初始化之前必须通过本机绑定、内网或防火墙限制访问。初始化完成后安装入口关闭；数据库故障不会重新开放安装。

浏览器使用本地存储的 Bearer 令牌，数据库只保存令牌哈希；有效使用会延长 30 天的空闲有效期。生产环境应通过 HTTPS 反向代理访问，并保护页面免受跨站脚本攻击，因为注入脚本可以读取浏览器本地存储中的令牌。客户端可使用同一套令牌接口。

开发前端时可在另一个终端运行 `cd frontend && npm run dev`，Vite 将 `/api` 代理到本机 8080 端口。完成调试后停止两个进程。

## 容器运行

使用 `compose.yaml`，选择一个数据库配置；Redis 可通过额外 profile 启用。请在运行前通过环境或私有 `.env` 文件设置 `DB_PASSWORD`，可单独设置 `DB_ROOT_PASSWORD`。示例仅绑定本机端口，避免将未初始化的服务直接暴露到公网。

本机可以通过 HTTP 试用；生产环境应使用 HTTPS，避免令牌在网络中被窃取。

```bash
docker compose --profile postgres up -d --build
docker compose --profile mysql up -d --build
docker compose --profile postgres --profile redis up -d --build
docker compose down
```

以上三条 `up` 命令是互斥的部署示例，选择其中一条。容器内的数据库主机分别填写 `postgres` 或 `mysql`；启用 Redis 时地址填写 `redis:6379`。应用配置与数据库均使用 Docker 卷持久化。迁移数据库或恢复备份时，应同时保留应用数据卷，禁止删除该卷后直接重开安装向导。

直接运行时默认只监听 `127.0.0.1:8080`；容器内监听 `:8080`，Compose 将端口绑定到宿主机本地。默认工作目录由 `LIBREFANZA_DATA_DIR` 控制。也可设置 `LIBREFANZA_LISTEN`、`LIBREFANZA_FRONTEND_DIR`、`FANZA_API_ID`、`FANZA_AFFILIATE_ID`、`PROXY_URL`、`FC2_BASE_URL`、`MGSTAGE_SERIES_URL` 和 `BLACKHOLE_PATTERN`。管理员界面保存的来源配置优先于对应环境默认值。日志不打印数据库连接密码或来源 API 密钥。

默认不信任转发的客户端 IP；反向代理部署时，可通过 `LIBREFANZA_TRUSTED_PROXIES` 设置可信代理 IP 或 CIDR，多个值用逗号分隔。仅填写自己控制的代理地址，以便登录和初始化限速识别真实客户端；不应信任所有来源。

## 运行日志

管理员可在“管理 → 运行日志”按级别、来源或完整任务编号筛选，查看采集开始、完成、每次失败和重试，以及来源检索失败。日志包含失败阶段、页码或编号、HTTP 状态和安全分类后的原因；单个来源失败后其他来源仍可继续检索。任务日志保存在数据库中，保留 30 天，后台每天清理过期记录。

任务日志同时以 JSON 行写入容器标准输出；启动失败或数据库不可用时也可从容器日志排查。数据库未连接之前的日志以及日志写库失败信息不会出现在管理页面。容器日志保留时间由 Docker 日志驱动和宿主机配置决定，不受上述 30 天限制。

```bash
docker compose logs --tail=200 app
docker compose logs -f app
```

本功能产生的任务日志不记录配置密钥、Cookie、请求体、来源响应正文或原始请求地址。为避免泄露敏感数据，无法安全分类的任务异常显示为未知原因，不会直接输出异常原文。

## 功能与接口

- 影片、演员浏览与筛选，来源版本、演员作品及单体作品查看。
- 未收录番号先按来源顺序检索并尽快返回第一个匹配项，随后后台补充其他来源；24 小时内成功取得且仍存在的来源记录不会重复请求。历史影片自动分批重新解析并补采；详情页汇总来源字段和图片，也可切换单个来源。播放器按需另行查找 Jable 播放来源。
- 管理员可调整封面和来源优先级、重新解析来源、查看原始页面、配置采集与 Redis，并查看和重试任务。原始 HTML 以文本形式显示。
- 定时采集默认使用 `America/Los_Angeles` 时区，演员每天 00:00 采集，已启用的 Fanza 和 MGStage 影片来源每 30 分钟采集；管理界面可设置分钟间隔、时区、Cron 表达式及启用状态。
- 新增来源的定时采集默认关闭，可在“定时任务”中启用。支持手动采集近期更新、全历史、指定页码、发行日期或番号范围；Jable 暂不支持发行日期范围。日期范围扫描结果中无发行日期的条目会被跳过并计数。全历史按源站列表顺序分页，并在失败、重试或工作进程恢复时从上一已完成页重新扫描；源站列表变动可能导致遗漏，不能保证历史数据完整。
- 来源默认地址分别为 `https://jable.tv/`、`https://avmoo.shop/`、`https://www.javlibrary.com/` 和 `https://www.libredmm.com/`，可在来源配置中覆盖。来源可达性、页面结构和站点验证可能变化；HTTP 403 等错误会在采集管理中显示。全历史任务会产生大量请求，同一数据库的多个实例共享来源限流，同一来源请求至少间隔 1 秒；429 响应遵守 `Retry-After`。
- 如已持有对应站点允许使用的有效会话，可在来源配置中设置 Jable、JavLibrary 的 Cookie 请求头值；留空保留原值，也可勾选清除。会话保存在应用数据库中，管理接口不返回其内容，应限制数据库和管理员访问。会话可能过期，也不能保证满足站点的其他访问验证条件。
- 影片详情可按需寻找 Jable 的 HLS 播放地址，优先由浏览器直连，失败时尝试服务端转发；不会下载或长期保存视频文件。转发仅允许公网 HTTPS 地址，阻止内网、IP 直连和不安全重定向；当前转发不使用来源配置中的 HTTP 代理。播放地址和转发令牌可能过期，重新查找可刷新；重启或多实例切换后须重新查找播放来源。
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
