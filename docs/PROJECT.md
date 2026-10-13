# 客服系统项目总纲

## 工程结构
```
kefu/
  server/           Go 后端 (Gin + GORM + WebSocket)，端口 8080
  web/              Vue3 + Vite + TS + Element Plus 客服工作台 (端口 5173)
  docs/API.md       ★ 前后端唯一契约，所有字段以此为准
```

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
- 请求封装 `web/src/api/http.ts`：baseURL `/api`，dev 下 vite proxy 转发到 `http://127.0.0.1:8080`；拦截器返回 `res.data.data`，`code!=0` 抛错
- 状态管理 Pinia：`stores/auth.ts`（token/user）、`stores/chat.ts`（会话/消息/未读）
- WebSocket：`web/src/composables/useSocket.ts`，`ws://<host>/ws?token=xxx`，自动重连
- 组件目录：`components/` 通用组件，`views/` 页面
- 图标用 `@element-plus/icons-vue`；渠道图标可用 emoji 或自定义 svg 占位

## 后端约定
- 统一响应 `{code, message, data}`，HTTP 恒 200
- 鉴权 `Authorization: Bearer <token>`
- SQLite 默认，MySQL 可切（改 config.yaml）
- 种子数据：`POST /api/seed` 或启动时自动初始化演示数据
