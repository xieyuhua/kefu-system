# 客服系统（Golang + Vue3）

前后端分离的 SAAS 客服系统：多渠道接入、客服工作台、子账号分渠道接待、客户标签、
历史订单/会话档案、自动回复与关键词转人工、数据统计，并预留 APP / PC 桌面端推送与离线消息同步。

```
kefu/
├─ server/          Go 后端（Gin + GORM + WebSocket + Redis 可选）   端口 8480
├─ web/             Vue3 + Vite + TS + Element Plus 前端            端口 5173
└─ docs/
   ├─ API.md        前后端唯一契约（所有接口/字段以此为准）
   └─ PROJECT.md    项目总纲与视觉规范
```

## 一键启动（Windows）

```powershell
.\start.ps1        # 停止旧进程 → 编译 → 后台启动 → 健康检查 → 打印三端地址
.\stop.ps1         # 停止后端
```

启动后访问（根路径 / 是空白页，属设计如此）：

| 入口 | 地址 | 默认账号 |
|---|---|---|
| 管理后台 | http://127.0.0.1:8480/admin/ | admin / admin123 |
| 客服工作台 | http://127.0.0.1:8480/chat/ | kf01 / 123456 |
| 用户端 | http://127.0.0.1:8480/kefu/?channel=wechat&uid=demo1 | — |

> 端口被占用时会给出明确提示；换端口用 .\start.ps1 -Port 8481。
## 文档

- **[docs/部署教程.md](docs/部署教程.md)** —— 部署上线：单机独立部署 / 分布式部署（构建、配置、systemd、Nginx、Redis、滚动升级、排错）
- **[docs/使用教程.md](docs/使用教程.md)** —— 从零开始的使用指引（启动、配置渠道/子账号、接待会话、接入用户端、离线消息）
- **[docs/推送对接.md](docs/推送对接.md)** —— APP / PC 桌面端推送对接（设备注册、厂商通道接入、离线补偿）
- [docs/API.md](docs/API.md) —— 前后端接口契约
- [docs/PROJECT.md](docs/PROJECT.md) —— 项目总纲与视觉规范

## 快速开始

### 前端：构建产物直接输出到 server/web

`vite.config.ts` 里 `build.outDir` 已指向 `../server/web`，**不需要再手动 cp**：

```bash
cd web
pnpm install      # 首次会自动创建 server/web 目录
pnpm build        # 产物直接落在 server/web（含 index/admin/chat/kefu 四个入口）
```

### 后端：启动后三个端都可直接访问

```bash
cd server
go mod download
go run .          # 首次启动自动建表 + 写入演示数据
```

| 地址 | 端 |
|---|---|
| `http://127.0.0.1:8480/` 或 `/kefu/` | 用户端（客户聊天页） |
| `http://127.0.0.1:8480/chat/` | 客服工作台 |
| `http://127.0.0.1:8480/admin/` | 管理后台 |

前端产物目录按以下顺序定位，**从任何目录启动都能找到**：
1. 环境变量 `KEFU_WEB_DIR`
2. 可执行文件同级目录下的 `web/`
3. 当前工作目录下的 `web/`

开发调试时用 `pnpm dev`（Vite 开发服务器，代理到后端），不需要先 build。

> ⚠️ 若本机 8480 端口被占用，改用其他端口：
> ```bash
> cd server && KEFU_PORT=8481 go run .
> ```
> 前端 build 时用 `VITE_PROXY_TARGET` 只影响 dev 代理，不影响 build 产物。

### 三端独立入口（重点）

前端是**多页应用（MPA）**：三个端各自有独立的入口 HTML、独立的 router、独立的打包 chunk，可单独部署与鉴权，共用同一套登录态（token 存 localStorage，登录一次三端通用）。

| 入口 | 端 | 入口文件 | 登录要求 |
|---|---|---|---|
| `/` 或 `/kefu/` | **用户端**（客户聊天页） | `index.html` / `kefu.html` | 不需要 |
| `/chat/` | **客服工作台** | `chat.html` | 登录（管理员与客服都可） |
| `/admin/` | **管理后台** | `admin.html` | 登录 + 管理员角色 |

- 三端内部都用 hash 路由：`/admin/#/channels`、`/chat/#/history`
- 旧地址自动跳转：`/c` → `/kefu/`，`/workbench` → `/chat/`
- 无尾斜杠会自动补：`/admin` → `/admin/`

构建产物结构（**直接生成在 `server/web/`**）：
```
server/web/
├─ index.html    # 根路径用户端
├─ kefu.html     # /kefu/ 用户端
├─ chat.html     # /chat/ 工作台
├─ admin.html    # /admin/ 后台
└─ assets/
   ├─ admin-*.js # 后台独立 chunk
   ├─ chat-*.js  # 工作台独立 chunk
   └─ ...
```

常用脚本：
```bash
pnpm dev        # 开发服务器（含热更新，代理 /api、/ws 到后端）
pnpm build      # 类型检查 + 构建到 ../server/web
pnpm build:only # 跳过类型检查，只构建（更快）
pnpm typecheck  # 只做类型检查
pnpm verify     # 跑响应式布局逻辑自测
```

用 Nginx 分离部署时，把 `server/web` 整个目录作为静态根目录即可（Nginx 配置见使用教程 §9）。

### 演示账号
| 账号 | 密码 | 角色 | 可接待渠道 |
|---|---|---|---|
| `admin` | `admin123` | 管理员 | 全部 |
| `kf01` | `123456` | 客服小李 | 微信客服、网页客服 |
| `kf02` | `123456` | 客服小王 | 企业微信、APP 客服、机器人助手 |

## 功能清单

### 客服工作台
- 三栏布局：渠道导航条 → 会话列表 → 聊天区 → 客户信息栏
- **来源渠道**：列表每条会话显示渠道图标+名称，侧边栏可按渠道筛选、看各渠道待接待角标
- 消息类型：文本 / 图片 / **订单卡片** / **商品卡片** / 系统通知
- 操作：接入（claim）、转接（transfer）、结束（close）、标记已读
- 实时：WebSocket 推送新消息/新会话/会话变更，断线自动重连
- 演示面板：一键模拟新客户咨询（`POST /api/demo/incoming`）

### 客户信息与数据统计
- **IP + 归属城市**：会话列表、客户资料栏、历史档案均展示客户 IP 与所在城市（需在 `config.yaml` 的 `ip_lookup.url` 配置查询服务，内网 IP 不解析）
- 客服可给客户**打标签**（租户级标签库），其他客服接待同一客户时可见
- 客户备注（remark）同样跨客服共享
- 历史档案：客户详情含**历史订单**列表与**历史会话**记录，点订单可直接以卡片形式发到聊天

### SAAS 多租户与子账号
- 租户下多个子账号，子账号可**多选"可接待渠道"**（`agent_channels`）
- 子账号字段：账号、姓名、角色(admin/agent)、最大并发接待数、在线状态、**启用/禁用**
- 子账号只看自己渠道的会话；管理员看全部

### 渠道
- **渠道类型完全自定义、不限制枚举**：内置 10 种建议类型，租户可在「渠道类型库」任意增删改（支持纯中文类型名）
- 每个渠道可独立开关「启用」与「机器人自动回复」

### 自动回复 / 转人工
- 关键词规则：`contain` 包含 / `equal` 完全匹配 / `prefix` 前缀，多关键词逗号分隔，按优先级匹配
- 支持按渠道生效或全渠道生效
- **`transfer_to_human` 转人工**：命中即把会话置为待接待、清空客服、插系统消息，并通知该渠道所有客服
- 命中次数统计（hit_count）

### 数据统计
- `GET /api/stats/overview?days=7`：最近 N 天接待趋势（按日的会话数/已接待/已结束/消息数）
- 客服工作量、渠道分布、汇总指标（接待率、平均消息数、新增客户等）

### 实时通道与离线补偿

两端统一 WebSocket，各自独立通道：

| 端 | 连接 |
|---|---|
| 客服端 | `ws://host/ws?token=<jwt>&device=web\|desktop\|app` |
| 用户端 | `ws://host/ws/guest?channel=web&uid=u_10001&since_id=0` |

- **消息一律先入库再推送**，所以客服或用户任一方离线都不影响消息送达
- **重连自动补拉**：连接时带 `since_id`（本地已收到的最大消息 ID），服务端回放之后累积的消息
- 前端指数退避重连（1s → 15s）；用户端连续失败 3 次自动降级为 3s 轮询，恢复后自动切回
- 分布式部署时通过 Redis Pub/Sub（`events` / `guest-events`）跨实例投递，访客连到任意实例都能收到

### 多端推送与离线消息（预留）
- **WebSocket**：Web 工作台 / PC 桌面端 / APP 共用 `GET /ws?token=&device=web|desktop|app`
- **设备注册**：`POST /api/devices` 登记推送 Token；`service/push.go` 是厂商推送网关占位实现（接 APNs / 极光 / 个推在此替换即可）
- **离线消息**：消息带全局自增 ID，端侧重连后用 `GET /api/offline/messages?since_id=` 增量补拉
- **离线提醒**：`GET /api/offline/notifications?since_id=` 拉取离线期间堆积的系统提醒
- 离线期间消息照常入库，客服上线后先增量同步再进实时通道

### 用户端（客户端）
- 入口 `/c`，**所有对接参数走 GET query**：`channel` `uid` `nickname` `avatar` `phone` `order_no` `pid` `scene` `visitor_id`
- 例：`/c?channel=web&uid=u_10001&nickname=王小明&pid=P10086`
- 页面风格参考**企业微信客服**：浅灰会话底、右侧绿色气泡、白色圆角订单/商品卡片、居中灰色系统 pill
- 接口：`GET /api/guest/init`、`POST /api/guest/message`、`GET /api/guest/messages`（无 WS 时降级轮询）

## 分布式部署（Redis）

Redis **可选**：不配置时自动降级为进程内内存实现，单机部署零依赖。

```yaml
# server/config.yaml
redis:
  enabled: true
  addr: 127.0.0.1:6379
  password: ""
  db: 0
  prefix: "kefu:"
```
或用环境变量：`KEFU_REDIS_ENABLED=1 KEFU_REDIS_ADDR=127.0.0.1:6379 go run .`

启用后：
- **Pub/Sub 跨实例广播**：A 实例收到的消息会广播到 B、C 实例的 WebSocket 连接（`internal/cache` + `service/hub.go`）
- **Token 缓存与黑名单**：JWT 校验走缓存，退出登录/强制下线多实例立即生效（`internal/auth`）
- 多实例部署时，前端需保证同一用户会话粘滞或用共享 Redis 做会话分发

## 切换 MySQL

编辑 `server/config.yaml`：
```yaml
database:
  driver: mysql
  host: 127.0.0.1
  port: 3306
  user: root
  password: root
  name: kefu
```
首次启动自动建表（GORM AutoMigrate）并写入演示数据。

## 目录结构（后端）

```
server/
├─ main.go
├─ config.yaml
└─ internal/
   ├─ config/     配置（含极简 YAML 解析 + 环境变量覆盖）
   ├─ db/         GORM 初始化（sqlite/mysql）+ 自动迁移
   ├─ model/      数据模型
   ├─ dto/        出入参结构
   ├─ cache/      Redis/内存双实现（Pub/Sub + 缓存），自动降级
   ├─ auth/       JWT 签发/校验/黑名单
   ├─ middleware/ JWT 鉴权、CORS
   ├─ service/    业务逻辑（会话/消息/标签/自动回复/统计/推送网关）
   ├─ api/        HTTP handlers
   ├─ router/     路由
   └─ seed/       演示数据
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `KEFU_PORT` | 监听端口，默认 8480 |
| `KEFU_MODE` | debug / release |
| `KEFU_DB_DRIVER` | sqlite / mysql |
| `KEFU_DB_DSN` | SQLite 文件路径 |
| `KEFU_DB_HOST/USER/PASSWORD/NAME` | MySQL 连接 |
| `KEFU_JWT_SECRET` | JWT 密钥 |
| `KEFU_REDIS_ENABLED/ADDR/PASSWORD/DB` | Redis 配置 |

## 已知限制 / 后续可接入
- 厂商推送（APNs/极光/个推）为占位实现，需在 `service/push.go` 接真实 SDK
- 图片上传：`MessageInput` 暂用 dataURL，接真实对象存储后可换 `/uploads` 静态目录
- 真实渠道回调（微信/抖音等）未接入，演示入口为 `/api/demo/incoming` 与 `/api/guest/*`
