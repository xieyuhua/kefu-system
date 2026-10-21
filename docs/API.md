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
  "hide_header": false, "hide_debug": false, "hide_more_menu": false,
  "support_order_card": true, "support_product_card": true,
  "created_at": "2026-03-01 09:00:00",
  "agent_count": 3, "pending_count": 2    // 该渠道待接待数
}]
```
> `agent` 角色只返回自己可接待的渠道。
> `support_order_card` / `support_product_card`：渠道级**卡片能力开关**（后台「渠道管理」可配，默认 `true`）。
> 关闭后该渠道不能收发对应卡片：用户端隐藏入口、客服工作台入口置灰、服务端直接拦截。

### 设备推送自检
`POST /api/devices/test-push`
```json
{ "title": "推送自检", "body": "收到说明厂商通道已接通" }
```
向**当前登录账号**已注册的所有设备触发一次推送，用于验证厂商通道是否接通。结果看服务端日志 `[push]`。
> 厂商通道未启用时接口仍返回 `code: 0`，但不会实际下发（提醒仍会落库，端侧可补拉）。

### 渠道标识 app_id 与接入地址

每个渠道有唯一 `app_id`（随机生成，形如 `kefu_a1b2c3d4`），是接入链接里区分渠道来源的依据。

`GET /api/channels` 返回：
```json
{
  "id": 1, "type": "wechat", "name": "微信公众号客服",
  "app_id": "kefu_a1b2c3d4",
  "access_path": "/kefu/?channel=kefu_a1b2c3d4",
  "access_url": "https://kefu.example.com/kefu/?channel=kefu_a1b2c3d4"
}
```

`POST /api/channels` 时 `app_id` 可省略 —— 留空由后端自动生成随机值；填写则校验同租户内唯一。

`POST /api/channels/:id/reset-app-id` —— **重置**渠道标识（生成新随机值）：
```json
{ "app_id": "kefu_6bd7600b", "access_url": "https://kefu.example.com/kefu/?channel=kefu_6bd7600b" }
```
> ⚠️ 重置后旧链接与二维码立即失效，需重新分发。

**用户端渠道解析优先级**（`GET /api/guest/*`、`/ws/guest`）：
1. `app_id` 精确匹配 ← **推荐**，同类型多渠道靠它区分
2. 渠道类型（`wechat` / `web` …）← 兼容旧链接
3. 数字渠道 ID ← 兼容旧链接

> 服务端启动时会为历史渠道自动补齐空的 `app_id`（幂等，不影响已分发的链接）。

`GET /api/channels/:id/qr?size=320&base=<可选>`
```json
{
  "channel_id": 1, "channel_name": "微信公众号客服", "channel_type": "wechat",
  "url": "http://127.0.0.1:8480/kefu/?channel=wechat",
  "path": "/kefu/?channel=wechat",
  "base64": "data:image/png;base64,iVBORw0KGgo...",
  "size": 320, "file_name": "channel-wechat-微信公众号客服.png"
}
```
- `size`：256 / 320 / 512（默认 320）
- `base`：可选，覆盖 `config.yaml` 的 `server.public_base`，用于生成二维码里的完整地址

`GET /api/channels/:id/qr.png?size=320` —— 直接返回 PNG，带 `Content-Disposition: attachment`，用于前端下载。

> 渠道接入地址的完整形式 = 前端根地址 + `access_path`（如 `http://host/kefu/?channel=wechat`）。
> 后端根地址由 `config.yaml` 的 `server.public_base` 或环境变量 `KEFU_PUBLIC_BASE` 配置。

### 渠道接待账号快速设置

`GET /api/channels/:id/agents`
```json
[{ "id":2,"name":"小李","account":"kf01","avatar":"...","online":true,"enabled":true,"selected":true }]
```

`PUT /api/channels/:id/agents`  `{ "user_ids": [2,3] }` —— **全量覆盖**该渠道的接待账号，返回同上数组。

### `POST /api/channels`（admin）
请求：`{ "type":"wechat", "name":"微信公众号客服", "app_id":"wx123", "enabled":true, "robot_enabled":false,
"support_order_card":true, "support_product_card":true }`
> `support_order_card` / `support_product_card` 省略时按 `true` 处理（与存量渠道一致）。

### `PUT /api/channels/:id`（admin）
请求体同 POST（字段可选）。
> 两个卡片开关是 `*bool` 语义：**不传表示不修改**，传 `false` 即关闭；可只传其中一个，另一个保持原值。

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
    "channel": {"id":1,"type":"wechat","name":"微信公众号客服",
                "support_order_card":true,"support_product_card":true},
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
> `sender.role`：`user`=客户、`agent`=真实客服账号（头像取账号自身）、`robot`=智能助手（机器人/自动回复）。
> **`robot` 消息的 `sender.avatar` 取后台设置 `assistant.avatar`**（未配置时为空串，前端回退内置头像），
> 因此后台改「助手头像」后，用户端与客服工作台两边的机器人气泡头像同时生效。

### `POST /api/conversations/:id/messages`
请求：`{ "type": "text", "content": "您好，很高兴为您服务" }`
响应 `data` = 新建的消息对象（用于乐观更新）。

> **渠道卡片能力强校验**：会话所属渠道关闭了对应卡片能力时，`type=order` 返回业务错误
> `该渠道未开启订单卡片`，`type=product` 返回 `该渠道未开启商品卡片`（HTTP 200 + `code: 400`，消息不落库）。
> 机器人自动回复产生的卡片不经过此校验。

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
http(s)://<host>/kefu/?channel=web&uid=o_abc123&nickname=王小明&avatar=&order_no=SO2026...&scene=product&pid=P10086&visitor_id=v_8899&sign=xxxx
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
/kefu/?channel=web&uid=u_10001&nickname=%E7%8E%8B%E5%B0%8F%E6%98%8E&scene=product&pid=P10086
```

> 旧的 `/c?channel=xxx` 仍可用，后端会 302 跳到 `/kefu/?channel=xxx`（向后兼容）。

### 13.2 初始化接口（用户端页面加载时调用）
`GET /api/guest/init?channel=web&uid=u_10001&nickname=...&order_no=...&pid=...`

响应 `data`：
```json
{
  "channel": {
    "id":3, "type":"web", "name":"网页在线客服",
    "hide_header": false, "hide_debug": false, "hide_more_menu": false,
    "support_order_card": true, "support_product_card": true
  },
  "customer": { "id":12, "external_id":"u_10001", "nickname":"王小明", "avatar":"..." },
  "conversation": { "id":31, "status":"pending" },
  "greeting": "您好，我是智能客服助手，请问有什么可以帮您？",
  "assistant_avatar": "/uploads/20261020/1792487352_xxx.png",
  "order":  { ...订单卡片 JSON 对象或 null... },
  "product": { ...商品卡片 JSON 对象或 null... },
  "history": [ ...最近 20 条消息... ]
}
```
> 页面按 `greeting` 直接渲染一条机器人欢迎语；若带 `order_no`/`pid`，自动渲染订单/商品卡片。
> `channel.support_order_card` / `channel.support_product_card` 为 `false` 时，用户端应**隐藏**对应卡片入口
> （含自动弹出的卡片确认气泡）；此时即便强行调用发消息接口也会被服务端拦截。
> `assistant_avatar`：智能助手（机器人）回复时展示的头像，取自后台设置项 `assistant.avatar`
> （后台「系统设置 → 助手头像」可上传/改 URL）。**未配置时是空串**，用户端据此回退到内置头像；
> 真实客服账号的消息仍用各自 `sender.avatar`，不受该字段影响。

### 13.3 用户端发消息
`POST /api/guest/message?channel=web&uid=u_10001`
请求体：`{ "type":"text", "content":"我想问下发货时间" }`
响应 `data`：
```json
{ "message": {...}, "reply": {...机器人回复，可能为 null...}, "transferred": false }
```
- `reply`：命中自动回复规则时的机器人回复（可能是文本或商品卡片）
- `transferred=true`：命中"转人工"关键词，会话已转为待人工接待，前端提示"正在为您转接人工客服"
- 卡片能力强校验：渠道关闭了订单/商品卡片时，`type=order|product` 返回 `code: 400` +
  `该渠道未开启订单卡片` / `该渠道未开启商品卡片`，消息不落库。

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

> 这组端点是**团队共用**的：后台「自动回复」页与客服工作台「自动回复管理」打同一组 method+path。
> 归属字段 `user_id`：`0` = 管理员建的公共规则（客服可见可用、只读）；`>0` = 该客服创建的规则。
> 权限：客服可新增（归属自己）与改 / 删**自己创建的**；动公共规则返回
> `公共规则仅管理员可修改`，动别人的规则返回 `只能修改自己创建的规则`；管理员可改全部。
> 匹配 / 转人工 / 命中计数的触发逻辑不受归属影响。
>
> **渠道是多选的**（`channel_ids`）：规则只对所选渠道的会话生效。
> 客服只能选**自己被授权的渠道**（`GET /api/auth/me` 的 `data.channels`），
> 传未授权渠道返回 `无权在该渠道配置自动回复（渠道 #x 未授权给你）`，
> 不选渠道返回 `请选择你已授权的渠道`；管理员不选 = 全部渠道。
> 列表可见范围不变：客服仍能看到全租户的规则（团队共用），只限制新增 / 修改时的可选渠道。

### `GET /api/auto-replies`
```json
[{
  "id":1, "name":"问候语", "user_id":0, "owner":"公共", "shared":true, "editable":false,
  "channel_id":0, "channel_name":"微信公众号客服、网页在线客服",
  "channel_ids":[1,3], "channel_names":["微信公众号客服","网页在线客服"],
  "match_type":"contain", "keywords":"你好,在吗,hi",
  "reply":"您好，我是智能客服助手～", "reply_type":"text",
  "transfer_to_human":false, "enabled":true, "priority":10, "hit_count":23
}]
```
> `channel_ids` 是多选渠道（已解析成数组，空数组 = 全部渠道），`channel_names` 与之一一对应；
> 旧字段 `channel_id` / `channel_name` 保留兼容：`channel_id = 0` 表示全部渠道，
> `channel_name` 在多渠道时是「、」连接的渠道名。
> 历史数据（只写过 `channel_id`）的规则按 `channel_id` 旧语义继续命中，不需要迁移。
> `match_type`：`contain` 包含 / `equal` 完全匹配 / `prefix` 前缀；
> `shared` 等价于 `user_id = 0`，`editable` 由后端按当前登录账号算好，前端据此置灰按钮。

### `POST /api/auto-replies`
```json
{ "name":"转人工", "channel_ids":[1,3], "match_type":"contain",
  "keywords":"人工,转人工,投诉", "reply":"", "transfer_to_human":true,
  "enabled":true, "priority":100 }
```
> 客服调用时归属写当前账号；管理员调用时归属写 `0`（公共规则）。
> `channel_ids` 为空（或旧客户端传 `channel_id = 0`）表示全部渠道，**只有管理员**可以这么传。
> `transfer_to_human = true` 时命中即**转人工**：会话置为 `pending`、清空 agent、
> 插入一条 system 消息，并通知该渠道所有客服（在线走 WebSocket，离线走 APP/PC 推送）。

### `PUT /api/auto-replies/:id`
### `DELETE /api/auto-replies/:id`
> 修改时同样校验渠道归属（防止先建后改绕过）；请求体不带 `channel_ids` / `channel_id` 时渠道保持原值。

---

## 14.1 快捷回复（常用语）

工作台输入框的「⚡ 快捷回复」弹层数据来自这里；`user_id` 为 `0` 的是**租户公共话术**（管理员维护，
所有客服可见、只读），`>0` 的是该客服的私有话术。

### `GET /api/quick-replies?keyword=`
返回「我的私有 + 租户共享」，按 `sort, id` 升序；`keyword` 可选（按 `title` / `content` 模糊）。
```json
[{
  "id":3, "user_id":0, "title":"问候语", "content":"您好，很高兴为您服务～",
  "sort":10, "enabled":true, "shared":true, "editable":false,
  "owner":"公共", "created_at":"2026-10-20 17:37:58"
}]
```

### `POST /api/quick-replies`
```json
{ "title":"核实订单", "content":"麻烦提供一下订单号，我这边帮您核实。", "sort":10, "enabled":true }
```
> `user_id` 取当前登录账号（**忽略**前端传值）。

### `PUT /api/quick-replies/:id` / `DELETE /api/quick-replies/:id`
> 只能改 / 删自己的；改别人的或共享项返回 `只能修改自己的快捷回复` / `只能删除自己的快捷回复`。

### 后台端（管理员维护租户全部话术）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/admin/quick-replies?keyword=` | 租户全部（含各客服私有项） |
| POST | `/api/admin/quick-replies` | 新建，默认落为公共话术（`user_id = 0`），可传 `user_id` 指定归属 |
| PUT | `/api/admin/quick-replies/:id` | 改租户内任意一条 |
| DELETE | `/api/admin/quick-replies/:id` | 删租户内任意一条 |

> 非管理员调用返回 `code: 403` + `仅管理员可操作`。
> 路径说明：`/quick-replies` 已由客服端分组占用，且 `/quick-replies/:id` 与任何
> `/quick-replies/xxx` 静态段在 gin 路由树里会冲突，因此后台侧挂在 `/admin/` 前缀下。

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

## 16. AI 应答（可选，默认关闭）

未命中自动回复规则的客户消息，按 **命中规则 → AI（检索该渠道自动回复内容 + 大模型）→ 兜底话术** 的顺序应答。
AI 相关配置全部存在 `settings` 表（一个 key 一个值），读写复用下面的通用设置接口；
检索语料就是后台「自动回复」页维护的规则（关键词 + 回复内容，按当前会话渠道过滤）。配置说明见 [AI应答.md](./AI应答.md)。

### 16.1 设置项（`GET /api/settings` / `PUT /api/settings`）

| key | 说明 | 默认值 |
|---|---|---|
| `ai.enabled` | 是否启用 AI 应答 | `false` |
| `ai.base_url` | OpenAI 兼容接口根地址（不含 `/chat/completions`），如 `https://api.deepseek.com/v1` | 空 |
| `ai.api_key` | 接口密钥；读取时脱敏为 `******`，回写 `******` 表示沿用原值 | 空 |
| `ai.model` | 模型名，如 `deepseek-chat` | 空 |
| `ai.system_prompt` | 系统提示词（默认要求「只依据自动回复内容回答，不确定就转人工」） | 见下 |
| `ai.timeout_seconds` | 单次请求超时（秒） | `20` |
| `rag.top_k` | 检索返回条数（拼进提示词的片段数） | `3` |
| `rag.min_score` | 最低相关度（0~1），低于该值视为无命中 | `0.15` |

```json
// GET /api/settings 片段（密钥已脱敏）
[
  { "key": "ai.enabled", "value": "true", "remark": "是否启用 AI 应答：……" },
  { "key": "ai.api_key", "value": "******", "remark": "大模型接口的 API Key；……" },
  { "key": "rag.min_score", "value": "0.15", "remark": "最低相关度（0~1）：……" }
]
```

> `ai.api_key` 与对象存储的 `secret_key` 同一套脱敏口径：接口只下发 `******`，
> 保存时提交 `******`（或留空）表示保留库中原密钥，因此后台表单不需要回显真实 Key。

### 16.2 `POST /api/settings/ai/test`（admin）

用后台表单里的值做一次极短实调，验证「地址 + 鉴权 + 模型名」是否可用（不保存配置）。

```json
{ "base_url": "https://api.deepseek.com/v1", "api_key": "******", "model": "deepseek-chat", "timeout_seconds": 10 }
```
```json
{ "code": 0, "message": "ok", "data": { "ok": true, "message": "连接成功，模型可用" } }
```

> 与 `/api/settings/storage/test` 同一约定：成功失败都返回 `code: 0`，结果看 `data.ok` / `data.message`。
> `api_key` 传 `******` 或留空时沿用库中已保存的密钥；未传 `base_url` / `model` 时同样回退到已保存值。
> 非管理员返回 `code: 403` + `仅管理员可测试 AI 接口配置`。

### 16.3 应答优先级与降级（对客户端无感知）

1. **命中自动回复规则** → 用规则里的 `reply`（原行为，`hit_count` 照常累加）；
2. **AI 应答**（`ai.enabled=true` 且 `base_url`/`api_key`/`model` 齐全）→ 检索该渠道的自动回复规则：
   - 有命中（分数 ≥ `rag.min_score`）→ 拼提示词调模型，成功则用模型文本回复；
   - 无命中 / 调用失败（超时、401、5xx、解析失败）→ **降级**；
3. **兜底话术** `robot.fallback_reply`。

> 检索语料只取「启用中、且适用于当前会话渠道」的自动回复规则（含全渠道规则）；勾选「转人工」的规则不参与检索。
> 降级只会影响「这一条回复由谁生成」，客户端始终能收到一条回复（最差情况是兜底话术）。
> 渠道未开启机器人时整段不回复，与改造前一致。AI 回复同样以 `sender_role=robot` 落库、广播，文案不加任何「AI 生成」前缀。

---

## 12. 演示账号

| 账号 | 密码 | 角色 |
|---|---|---|
| `admin` | `admin123` | 租户管理员（云跳跳科技） |
| `kf01` | `123456` | 客服 小李（接待：微信/网页） |
| `kf02` | `123456` | 客服 小王（接待：企微/APP + 机器人） |
