# 客服系统项目总纲

## 工程结构
```
kefu/
  server/
    main.go              启动装配：config → db → cache → seed → route
    pkg/                 通用层（可复用、不依赖 internal）
      response           统一响应壳
      storage            GORM 连接/事务/分页
      cache              缓存抽象（Redis/内存降级，Init(Options)）
      httpx              通用 gin 辅助（参数解析、JSON 绑定）
      util               纯工具函数：时间格式化、密码哈希、IP 解析、随机串
    internal/
      admin/         后台端：handler/ service/ route/（无自有 model/repository，直接用 common）
      agent/         客服端：handler/ service/ route/
      client/        用户端：handler/ service/ route/
      common/        三端共用
        handler/ service/ repository/ model/ route/   业务与数据访问
        auth/ authctx/ card/ config/ db/ dto/ idgen/  基础设施（原 platform 已并入）
        ipinfo/ middleware/ realtime/ seed/ setting/
      route/router.go 组合入口：静态资源 + SPA + 三端路由汇总
  web/                   Vue3 + Vite + TS + Element Plus（三端独立入口）
    src/
      app/               应用引导：App.vue、bootstrap.ts、style.css
      shared/            三端共用：api/ types/ utils/ composables/ stores/auth
                        + views/LoginView.vue + components/(卡片/媒体/缩略图/个人资料)
      client/            ① 用户端（kefu.html）：ClientView + router
      chat/              ② 客服端（chat.html）：WorkbenchView + 工作台组件 + stores/chat
      admin/             ③ 后台端（admin.html）：AdminLayout + 10 个管理视图 + router/routes
      main-kefu.ts / main-chat.ts / main-admin.ts   三个 MPA 入口
  docs/API.md            ★ 前后端唯一契约，所有字段以此为准
  docs/架构分层.md        分层约定与新增接口的标准动作
```

后端按「业务模块优先、模块内技术分层」组织，依赖方向为
`platform ← account/channel/customer ← chat ← guest/automation/ops/upload`，
下层不得反向依赖上层。详见 [架构分层.md](架构分层.md)。

前端按「共用层 + 三端」组织：`shared/` 只能被三端引用，三端之间**不得互相引用**
（跨端跳转用整页 `window.location.href` 换入口，不能只改 hash）。

## 角色与权限
- `admin`：租户管理员，可管理渠道、子账号，可看全部会话
- `agent`：客服子账号，只能看自己可接待渠道 + 自己接待的会话

## 演示账号
| 账号 | 密码 | 角色 |
|---|---|---|
| admin | admin123 | 管理员 |
| kf01 | 123456 | 客服小李（微信/网页） |
| kf02 | 123456 | 客服小王（企微/APP/机器人） |

## 渠道类型 (channel.type)
`wechat` 微信客服 / `wecom` 企业微信 / `web` 网页 / `app` APP / `weibo` 微博 / `douyin` 抖音 / `robot` 机器人

## 视觉规范（重要）
整体参考**企业微信客服**风格：
- 布局：左侧窄导航条（渠道图标） + 中间会话列表 + 右侧聊天区 + 最右客户信息/订单栏
- 标题栏：白色/浅灰，居中显示客户名 + 渠道来源标签，右上角操作（转接/结束会话）
- 气泡：客户消息左侧白底；客服消息右侧 **企微绿 #07C160**；圆角 8px；字号 14px
- 系统通知：居中灰色 pill，字号 12px
- 订单/商品卡片：白色圆角卡片，左侧缩略图 48-64px，右侧标题+价格+状态
- 会话列表：白底，选中项浅灰 #E9EAEC 高亮，未读数红色圆点
- 输入框：白色工具条（表情/图片/订单/商品/快捷回复）+ 多行输入 + 右下角绿色发送按钮
- 主色：#07C160（绿）、文字 #191919 / #7F7F7F、分割线 #E5E5E5、背景 #F5F5F5 / #EDEDED

## 前端约定
- 路径别名 `@` → `web/src`
- 请求封装 `web/src/shared/api/http.ts`：baseURL `/api`，dev 下 vite proxy 转发到 `http://127.0.0.1:8480`；拦截器返回 `res.data.data`，`code!=0` 抛错
- 状态管理 Pinia：`shared/stores/auth.ts`（token/user）、`chat/stores/chat.ts`（会话/消息/未读，仅客服端）
- WebSocket：`web/src/shared/composables/useSocket.ts`，`ws://<host>/ws?token=xxx`，自动重连
- 目录归属：`shared/` 三端共用；`client/` 用户端；`chat/` 客服工作台；`admin/` 管理后台
- 图标用 `@element-plus/icons-vue`；渠道图标可用 emoji 或自定义 svg 占位

## 后端约定
- 统一响应 `{code, message, data}`，HTTP 恒 200
- 鉴权 `Authorization: Bearer <token>`
- SQLite 默认，MySQL 可切（改 config.yaml）；MySQL 必须 utf8mb4 —— 启动时会自动把库与既有表转为 utf8mb4（否则 emoji 等 4 字节字符报 Error 1366）
- 种子数据：`POST /api/seed` 或启动时自动初始化演示数据
