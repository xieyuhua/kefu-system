# 客服系统 API 契约

统一前缀：`/api`。除登录外的所有接口需要请求头：

```
Authorization: Bearer <token>
```

响应统一结构（**所有接口都包在 `data` 里**）：

```json
{ "code": 0, "message": "ok", "data": { ... } }
```

- `code = 0` 成功；非 0 失败（`401` 未登录/过期，`403` 无权限，`400` 参数错误，`500` 服务端错误）。
- HTTP 状态码始终 200，业务错误看 `code`。

> 前端 axios 封装位置：`web/src/api/http.ts`。响应拦截器直接返回 `res.data.data`，并把 `code != 0` 抛成 Error（message 取 `message` 字段）。

---

## 1. 通用枚举

### 渠道类型 `channel.type`
**类型完全自定义、不限制固定枚举**。`type` 是任意字符串标识（建议英文/拼音，如 `wechat`、`xiaohongshu`、`offline_store`），展示名、颜色、图标由「渠道类型库」配置。

内置建议类型（首次访问自动写入类型库，可随意增删改）：

| code | 显示名 | 颜色 | 图标 |
|---|---|---|---|
| `wechat` | 微信客服 | #07C160 | 💬 |
| `wecom` | 企业微信 | #2E6BE6 | 🏢 |
| `web` | 网页客服 | #0BB2D4 | 🌐 |
| `app` | APP 客服 | #FF7D41 | 📱 |
| `weibo` | 微博 | #F5576C | 📕 |
| `douyin` | 抖音 | #191919 | 🎵 |
| `xiaohongshu` | 小红书 | #FF2442 | 📒 |
| `kuaishou` | 快手 | #FF6600 | ⚡ |
| `email` | 邮件 | #8B5CF6 | ✉️ |
| `robot` | 机器人助手 | #8B5CF6 | 🤖 |

> 前端渠道「类型」字段应为**可输入 + 预设下拉**（el-select `allow-create`），用户输入任意新类型时先调 `POST /api/channel-types` 创建，再创建渠道。

### 渠道类型库（租户自定义类型）
`GET /api/channel-types`
```json
[{ "id":1,"code":"wechat","name":"微信客服","color":"#07C160","icon":"💬",
   "sort":1,"built_in":true,"used_count":2,"created_at":"2026-03-01 09:00:00" }]
```
`POST /api/channel-types`  `{ "code":"xiaohongshu","name":"小红书","color":"#FF2442","icon":"📒","sort":7 }`
- `code` 留空时自动用 `name` 兜底（支持纯中文类型名）
- `PUT /api/channel-types/:id`
- `DELETE /api/channel-types/:id` —— 该类型下还有渠道时返回 400「该类型下还有渠道，无法删除」；`built_in` 类型前端应禁止删除

### 会话状态 `conversation.status`
| 值 | 含义 |
|---|---|
| `pending` | 待接待（在"未分配"池里） |
| `open` | 进行中（已分配/已接入） |
| `closed` | 已结束 |

### 消息类型 `message.type`
| 值 | 含义 | `content` 约定 |
|---|---|---|
| `text` | 文本 | 纯文本字符串 |
| `image` | 图片 | 图片 URL 字符串 |
| `order` | **订单卡片** | JSON 字符串，结构见下 |
| `product` | **商品卡片** | JSON 字符串，结构见下 |
| `system` | 系统通知（灰色居中 pill） | 纯文本 |

订单卡片 JSON：
```json
{
  "order_no": "SO20260305001",
  "title": "2026春季新款 纯棉圆领T恤",
  "status": "已发货",
  "amount": 199.00,
  "quantity": 2,
  "created_at": "2026-03-01 10:22:11",
  "image": "/demo/product-1.svg",
  "sku": "白色 / L"
}
```

商品卡片 JSON：
```json
{
  "id": "P10086",
  "title": "2026春季新款 纯棉圆领T恤",
  "price": 99.50,
  "image": "/demo/product-1.svg",
  "stock": 128,
  "sku": "白色 / L",
  "url": "https://example.com/p/P10086"
}
```

### 消息方向 `message.direction`
`in`（用户→客服）/ `out`（客服→用户）/ `system`

### 角色 `role`
`admin`（租户管理员）/ `agent`（客服子账号）

---

## 2. 认证

### `POST /api/auth/login`
请求：
```json
{ "account": "admin", "password": "admin123" }
```
响应 `data`：
```json
{
  "token": "eyJhbGciOi...",
  "user": {
    "id": 1, "tenant_id": 1, "role": "admin",
    "name": "张管理", "account": "admin", "avatar": "/demo/avatar-1.svg",
    "online": true,
    "channels": [1,2,3],          // 该账号可接待的渠道 ID；admin 为可管理的全部渠道
    "tenant": { "id": 1, "name": "云跳跳科技", "logo": "/demo/logo.svg" }
  }
}
```

### `GET /api/auth/me`
同上返回 `user` 对象。

### `POST /api/auth/logout`
`data: true`

---

## 3. 渠道

### `GET /api/channels`
响应 `data`：数组
```json
[{
  "id": 1, "type": "wechat", "name": "微信公众号客服",
  "app_id": "wx123456", "enabled": true, "robot_enabled": false,
  "created_at": "2026-03-01 09:00:00",
  "agent_count": 3, "pending_count": 2    // 该渠道待接待数
}]
```
> `agent` 角色只返回自己可接待的渠道。

### 设备推送自检
`POST /api/devices/test-push`
```json
{ "title": "推送自检", "body": "收到说明厂商通道已接通" }
```
向**当前登录账号**已注册的所有设备触发一次推送，用于验证厂商通道是否接通。结果看服务端日志 `[push]`。
> 厂商通道未启用时接口仍返回 `code: 0`，但不会实际下发（提醒仍会落库，端侧可补拉）。

### 渠道二维码与接入地址

`GET /api/channels/:id/qr?size=320&base=<可选>`
```json
{
  "channel_id": 1, "channel_name": "微信公众号客服", "channel_type": "wechat",
  "url": "http://127.0.0.1:5173/c?channel=wechat",
  "path": "/c?channel=wechat",
  "base64": "data:image/png;base64,iVBORw0KGgo...",
  "size": 320, "file_name": "channel-wechat-微信公众号客服.png"
}
```
- `size`：256 / 320 / 512（默认 320）
- `base`：可选，覆盖 `config.yaml` 的 `server.public_base`，用于生成二维码里的完整地址

`GET /api/channels/:id/qr.png?size=320` —— 直接返回 PNG，带 `Content-Disposition: attachment`，用于前端下载。

> 渠道接入地址的完整形式 = 前端根地址 + `access_path`（如 `http://host/c?channel=wechat`）。
> 后端根地址由 `config.yaml` 的 `server.public_base` 或环境变量 `KEFU_PUBLIC_BASE` 配置。

### 渠道接待账号快速设置

`GET /api/channels/:id/agents`
```json
[{ "id":2,"name":"小李","account":"kf01","avatar":"...","online":true,"enabled":true,"selected":true }]
```

`PUT /api/channels/:id/agents`  `{ "user_ids": [2,3] }` —— **全量覆盖**该渠道的接待账号，返回同上数组。

### `POST /api/channels`（admin）
请求：`{ "type":"wechat", "name":"微信公众号客服", "app_id":"wx123", "enabled":true, "robot_enabled":false }`

### `PUT /api/channels/:id`（admin）
请求体同 POST（字段可选）。

### `DELETE /api/channels/:id`（admin）
`data: true`

---

## 4. 子账号管理（admin）

### `GET /api/agents`
```json
[{
  "id": 2, "account": "kf01", "name": "小李", "role": "agent",
  "avatar": "/demo/avatar-2.svg", "online": false,
  "max_concurrent": 5, "status": "offline",
  "channels": [{"id":1,"type":"wechat","name":"微信公众号客服"}],
  "active_count": 0
}]
```

### `POST /api/agents`（admin）
请求：
```json
{ "account":"kf02", "password":"123456", "name":"小王", "max_concurrent": 5,
  "channel_ids": [1,2], "role": "agent" }
```

### `PUT /api/agents/:id`（admin）
同 POST，`password` 留空表示不修改；可传 `channel_ids` 覆盖该子账号的接待渠道。

### `DELETE /api/agents/:id`（admin）
`data: true`

### `POST /api/agents/:id/online`（自己或 admin）
`{ "online": true }` → `data: { "online": true, "status": "online" }`

---

## 5. 会话

### `GET /api/conversations`
Query：
| 参数 | 说明 |
|---|---|
| `channel_id` | 渠道 ID，0/缺省 = 全部 |
| `status` | `pending` / `open` / `closed` / `all`（缺省 all） |
| `scope` | `mine`（我的，缺省）/ `all`（全部，仅 admin 有效） |
| `keyword` | 搜索用户昵称或最后一条消息 |
| `page` / `page_size` | 分页，缺省 1 / 20 |

响应 `data`：
```json
{
  "list": [{
    "id": 11, "channel_id": 1,
    "channel": {"id":1,"type":"wechat","name":"微信公众号客服"},
    "user": {
      "id":3, "external_id":"wx_o_abc", "nickname":"王小明",
      "avatar":"/demo/avatar-3.svg", "phone":"138****8888",
      "ip":"203.0.113.45", "city":"广东省 深圳市",
      "tags":[{"id":1,"name":"VIP","color":"red"}], "remark":"老客户"
    },
    "status": "open",
    "agent": {"id":2,"name":"小李"},
    "unread": 2,
    "last_message": "这个什么时候发货？",
    "last_message_at": "2026-03-05 14:22:10",
    "created_at": "2026-03-05 14:00:00",
    "order_count": 3
  }],
  "total": 42, "page": 1, "page_size": 20
}
```

### `GET /api/conversations/:id`
`data` 为单个会话对象（字段同上，额外带 `user.order_count`）。

### `POST /api/conversations/:id/claim` 领取（接入）会话
`data` = 更新后的会话对象。已分配给别人时返回 `code=400`。

### `POST /api/conversations/:id/transfer`
请求 `{ "agent_id": 5 }` → `data` = 会话对象。

### `POST /api/conversations/:id/close`
`data` = 会话对象（status 变 `closed`，并插入一条 `system` 消息）。

### `POST /api/conversations/:id/read`
`data: true`（清空未读计数）

### `GET /api/conversations/stats`
```json
{ "pending": 5, "open": 3, "closed": 120, "unread": 8 }
```

---

## 6. 消息

### `GET /api/conversations/:id/messages`
Query：`before_id`（翻页用，返回更早的消息）、`limit`（缺省 50）。

响应 `data`：数组，按 id 升序```json
[{
  "id": 101, "conversation_id": 11, "type": "text", "direction": "in",
  "content": "你好，在吗？",
  "sender": {"id":3,"name":"王小明","avatar":"/demo/avatar-3.svg","role":"user"},
  "created_at": "2026-03-05 14:00:00"
}]
```
> `direction = system` 的消息在前端渲染为居中灰色 pill。
> `type = order|product` 时 `content` 为 JSON 字符串，前端需 `JSON.parse`。

### `POST /api/conversations/:id/messages`
请求：`{ "type": "text", "content": "您好，很高兴为您服务" }`
响应 `data` = 新建的消息对象（用于乐观更新）。

### `GET /api/conversations/:id/messages/count`
不需要。

---

## 7. 用户（客户）与历史档案

### `GET /api/users`
Query：`keyword`、`channel_id`、`page`、`page_size`
```json
{ "list": [{
    "id": 3, "external_id": "wx_o_abc", "nickname": "王小明",
    "avatar": "/demo/avatar-3.svg", "phone": "13800138000",
    "channel": {"id":1,"type":"wechat","name":"微信公众号客服"},
    "order_count": 5, "conversation_count": 4,
    "last_seen_at": "2026-03-05 14:22:10",
    "created_at": "2026-02-01 10:00:00"
  }], "total": 100, "page": 1, "page_size": 20 }
```

### `GET /api/users/:id`
单个用户 + `orders` 历史订单 + `conversations` 历史会话：
```json
{
  "user": { "id":3, "nickname":"王小明", "avatar":"...", "phone":"...", "channel": {...},
            "created_at":"2026-02-01 10:00:00", "tags":["VIP","高价值"] },
  "orders": [{
     "id":1, "order_no":"SO20260305001", "title":"2026春季新款 纯棉圆领T恤",
     "status":"已发货", "amount":199.00, "quantity":2, "image":"/demo/product-1.svg",
     "sku":"白色 / L", "created_at":"2026-03-01 10:22:11", "paid_at":"2026-03-01 10:23:00"
  }],
  "conversations": [{
     "id":11, "channel":{"id":1,"type":"wechat","name":"微信公众号客服"},
     "status":"closed", "agent":{"id":2,"name":"小李"},
     "message_count": 12, "summary":"咨询发货时间",
     "created_at":"2026-03-01 11:00:00", "closed_at":"2026-03-01 11:20:00"
  }]
}
```
历史订单点击可直接在聊天窗口以 `order` 卡片形式发送（前端用 `/api/conversations/:id/messages` 提交 `type=order`）。

---

## 8. 订单 & 商品

### `GET /api/orders`
Query：`user_id`、`keyword`（订单号/商品名）、`page`、`page_size`

### `GET /api/products`
Query：`keyword`、`page`、`page_size`
```json
{ "list": [{ "id":"P10086", "title":"...", "price":99.5, "image":"...", "stock":128, "sku":"白色 / L", "url":"..." }], "total": 20 }
```

---

## 9. 演示/模拟入口

### `POST /api/demo/incoming`
模拟用户在指定渠道发起新咨询（用于一键体验收发消息）。
请求：
```json
{ "channel_id": 1, "nickname": "王小明", "content": "你好，我想问一下发货时间", "type": "text" }
```
响应 `data` = 新建的会话对象。消息通过 WebSocket 实时推送到工作台。

### `GET /api/demo/products`
返回可发送的商品卡列表（等价于 `/api/products`）。

---

## 10. 实时推送与离线消息（Web / PC 桌面端 / APP）

系统设计上把「实时通道」抽象为多端网关，同一条消息会同时投递到在线的 Web、PC 桌面端、APP：
- **Web 工作台**：WebSocket `/ws?token=<jwt>`
- **PC 桌面端（Electron 预留）**：同样走 `/ws`，额外上报 `device` 标识
- **APP（预留）**：通过 `/api/devices` 注册推送 Token，服务端经 `gateway` 抽象层调用厂商推送（APNs / 极光 / 个推），离线时转成系统通知栏提醒

### 10.1 WebSocket
`GET /ws?token=<jwt>&device=web|desktop|app`

服务端下发 JSON：
```json
{ "event": "message.new", "data": { ...消息对象... } }
{ "event": "conversation.new", "data": { ...会话对象... } }
{ "event": "conversation.updated", "data": { ...会话对象... } }
{ "event": "notification.new", "data": { "id":1,"title":"新消息","body":"王小明：在吗","conversation_id":11 } }
```

前端 `useSocket.ts` 订阅后：
- `message.new` → 若 `conversation_id` 是当前会话则追加气泡；否则会话列表未读 +1、列表刷新。
- `conversation.new` / `conversation.updated` → 刷新会话列表与统计。
- `notification.new` → 弹桌面/页面通知提醒。

`robot` 渠道的消息会自动由服务端产生一条机器人回复（延迟 ~1s），同样通过 `message.new` 推送。

### 10.2 设备注册（APP / PC 桌面端）
`POST /api/devices`
```json
{ "device": "app", "platform": "ios", "push_token": "xxxxx", "name": "iPhone 15" }
```
响应 `data` = 设备对象 `{ "id":1,"device":"app","platform":"ios","name":"iPhone 15","created_at":"..." }`

`DELETE /api/devices/:id` 注销设备。
`GET /api/devices` 列出当前账号已注册设备。

### 10.3 离线消息同步（APP / PC 端断线重连后拉取）
`GET /api/offline/messages?since_id=0&limit=200`
返回自 `since_id` 之后的所有消息（含离线期间堆积的），按 id 升序：
```json
{ "list": [ { ...消息对象... } ], "latest_id": 1024, "has_more": false }
```
> 端侧本地持久化 `latest_id`，重连后以此为 `since_id` 增量拉取，保证离线消息不丢。

`GET /api/offline/notifications?since_id=0`
离线期间的系统提醒列表（新会话分配、转接、超时提醒等）：
```json
{ "list": [{ "id":3,"type":"conversation.assigned","title":"新会话分配",
             "body":"王小明 从 微信公众号客服 接入","conversation_id":11,
             "read":false,"created_at":"2026-03-05 14:00:00" }],
  "latest_id": 3 }
```

`POST /api/offline/notifications/read`  `{ "ids": [1,2,3] }` → `data: true`

### 10.4 离线时接收与回复
- 客服离线期间进入的消息照常入库（`message.direction = in`），会话 `unread` 累加，并进入「未接待/待处理」池。
- 客服重新上线后：先 `GET /api/offline/messages` 增量补齐，再进入 WebSocket 实时通道。
- 回复走 `POST /api/conversations/:id/messages`（HTTP，不依赖长连接），APP 弱网下可直接重试；成功后 WebSocket 会同步给其他端。
- 服务端为每条消息生成全局自增 `id`，是离线同步的唯一游标。

---

## 11. 静态资源

`/uploads/...` 静态目录（可选，图片上传）；`/demo/*.svg` 为内置演示图片（订单/商品/头像/logo）。

---

## 13. 用户端（客户端）接入 —— 参数通过 GET 传递

用户端页面在对接（网页客服、H5、APP WebView）时，**渠道等所有参数一律通过 URL query 传递**，
便于运营直接拼链接投放，无需前端二次开发。

### 13.1 用户端页面入口
```
http(s)://<host>/c?channel=web&uid=o_abc123&nickname=王小明&avatar=&order_no=SO2026...&scene=product&pid=P10086&visitor_id=v_8899&sign=xxxx
```

| 参数 | 必填 | 说明 |
|---|---|---|
| `channel` | 是 | 渠道标识，可用 **渠道类型**（`wechat`/`wecom`/`web`/`app`/`douyin`/`robot`）或渠道 ID |
| `uid` | 否 | 用户在业务侧的 openid / user_id；不传则自动生成游客 ID |
| `nickname` | 否 | 用户昵称，不传则用游客默认昵称 |
| `avatar` | 否 | 头像 URL |
| `phone` | 否 | 手机号（可选，便于客服回联） |
| `order_no` | 否 | 来源订单号，进入后自动带上该订单上下文 |
| `pid` | 否 | 来源商品 ID，进入后自动展示该商品 |
| `scene` | 否 | 自定义场景标识（如 `product`/`order`/`vip`），用于自动回复分流 |
| `visitor_id` | 否 | 游客追踪 ID（未登录用户） |
| `sign` | 否 | 签名（可选，防篡改，约定 md5(uid+channel+secret)） |

示例：
```
/c?channel=web&uid=u_10001&nickname=%E7%8E%8B%E5%B0%8F%E6%98%8E&scene=product&pid=P10086
```

### 13.2 初始化接口（用户端页面加载时调用）
`GET /api/guest/init?channel=web&uid=u_10001&nickname=...&order_no=...&pid=...`

响应 `data`：
```json
{
  "channel": { "id":3, "type":"web", "name":"网页在线客服" },
  "customer": { "id":12, "external_id":"u_10001", "nickname":"王小明", "avatar":"..." },
  "conversation": { "id":31, "status":"pending" },
  "greeting": "您好，我是智能客服助手，请问有什么可以帮您？",
  "order":  { ...订单卡片 JSON 对象或 null... },
  "product": { ...商品卡片 JSON 对象或 null... },
  "history": [ ...最近 20 条消息... ]
}
```
> 页面按 `greeting` 直接渲染一条机器人欢迎语；若带 `order_no`/`pid`，自动渲染订单/商品卡片。

### 13.3 用户端发消息
`POST /api/guest/message?channel=web&uid=u_10001`
请求体：`{ "type":"text", "content":"我想问下发货时间" }`
响应 `data`：
```json
{ "message": {...}, "reply": {...机器人回复，可能为 null...}, "transferred": false }
```
- `reply`：命中自动回复规则时的机器人回复（可能是文本或商品卡片）
- `transferred=true`：命中"转人工"关键词，会话已转为待人工接待，前端提示"正在为您转接人工客服"

### 13.4 实时通道与离线补偿

两端统一走 WebSocket，各自一条通道：

| 端 | 连接地址 | 鉴权 |
|---|---|---|
| 客服端 | `GET /ws?token=<jwt>&device=web\|desktop\|app` | JWT |
| **用户端（访客）** | `GET /ws/guest?channel=web&uid=u_10001&since_id=0&tenant=1` | channel + uid |

**用户端事件**
```json
{ "event": "connected", "data": { "conversation_id":31,"customer_id":12,"nickname":"王小明","channel":{...} } }
{ "event": "message.new", "data": { ...消息对象... } }
{ "event": "conversation.updated", "data": { "id":31,"status":"pending","transferred":false } }
```

**离线不丢消息的保证**：
1. 消息一律先入库，再推送 —— 对方在线即时收到，离线也不丢。
2. 任一端重连时带上 `since_id`（端侧已收到的最大消息 ID），**服务端自动回放这之后累积的消息**。
3. 访客端 `since_id` 存在 `localStorage`（key 按 `channel+uid` 隔离），换设备/清缓存后会由 `guest/init` 的 `history` 兜底。
4. 前端连续 3 次连接失败时**自动降级为轮询** `GET /api/guest/messages?since_id=`（3s 一次），连接恢复后自动切回 WebSocket。
5. 分布式部署时通过 Redis Pub/Sub（`events` 与 `guest-events` 两个 topic）跨实例投递，访客连到任意实例都能收到。

> 保留 `GET /api/guest/messages` 作为降级兜底，其 `since_id` 为**正向游标**：返回 `id > since_id` 的新消息。
> 注意与 `GET /api/conversations/:id/messages` 的 `before_id`（向前翻页，语义相反）区分。

---

## 14. 自动回复规则（关键词 → 机器人回复 / 转人工）

### `GET /api/auto-replies`
```json
[{
  "id":1, "name":"问候语", "channel_id":0, "channel_name":"全渠道",
  "match_type":"contain", "keywords":"你好,在吗,hi",
  "reply":"您好，我是智能客服助手～", "reply_type":"text",
  "transfer_to_human":false, "enabled":true, "priority":10, "hit_count":23
}]
```
> `channel_id = 0` 表示全渠道生效；`match_type`：`contain` 包含 / `equal` 完全匹配 / `prefix` 前缀

### `POST /api/auto-replies`（admin）
```json
{ "name":"转人工", "channel_id":0, "match_type":"contain",
  "keywords":"人工,转人工,投诉", "reply":"", "transfer_to_human":true,
  "enabled":true, "priority":100 }
```
> `transfer_to_human = true` 时命中即**转人工**：会话置为 `pending`、清空 agent、
> 插入一条 system 消息，并通知该渠道所有客服（在线走 WebSocket，离线走 APP/PC 推送）。

### `PUT /api/auto-replies/:id`（admin）
### `DELETE /api/auto-replies/:id`（admin）

---

## 15. 数据统计

### `GET /api/stats/overview?days=7`

`days` 支持 1-90，默认 7。

**统计口径**（页面上各区块共用同一时间范围）：
- `trend`：按天统计所选区间内**新建**的会话与消息
- `channels`：所选区间内**新建**的会话按渠道分布
- `agents.messages`：所选区间内该账号发送的客服消息数
- `agents.handled / closed / active_now`：**累计值**（不受 days 影响）
- `summary.total_conversations / customers / orders`：**累计值**
- `summary.new_customers`：所选区间内新增客户数

> 客服工作量表包含**管理员**——管理员若也亲自接待会话，其工作量会计入（`role: "admin"`）。
> 没有参与过接待且当前离线的账号不会出现在表中。

```json
{
  "trend": [{
     "date":"2026-03-05", "total":32, "handled":28, "closed":25,
     "pending":4, "messages":210, "customer_msgs":120, "agent_msgs":90
  }],
  "agents": [{ "agent_id":2,"name":"小李","role":"agent","handled":18,"closed":15,"messages":96,"active_now":3 }],
  "channels": [{ "channel_id":1,"name":"微信公众号客服","type":"wechat","total":12,"pending":2,"open":3,"closed":7 }],
  "summary": {
     "total_conversations": 128, "handled_rate": 87.5, "avg_messages": 6.4,
     "customers": 96, "new_customers": 12, "orders": 210, "online_agents": 2
  }
}
```

---

## 12. 演示账号

| 账号 | 密码 | 角色 |
|---|---|---|
| `admin` | `admin123` | 租户管理员（云跳跳科技） |
| `kf01` | `123456` | 客服 小李（接待：微信/网页） |
| `kf02` | `123456` | 客服 小王（接待：企微/APP + 机器人） |
