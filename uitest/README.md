# UI 自动化测试平台（Golang 服务端 + 浏览器插件录制）

面向后台业务操作（新建服务商品 / 上传商品 / 设置活动 …）的**录制 → AI 准备数据 → 自动执行 → 查看全过程**平台。

- **服务端（Go）**：内置浏览器（Edge / Chrome，走 CDP 驱动）、录制与运行存储、AI/规则数据生成、执行引擎、自带网页控制台。
- **客户端（浏览器插件）**：一键开始录制，把每一次点击/输入/选择/上传/跳转记成步骤，带**稳定 ID 绑定**、**中文备注**，实时上报服务端。
- **一个独立项目目录**：`uitest/`，自己的 `go.mod`、自己的端口（默认 **8490**），不改动仓库里其它任何代码。

```
uitest/
├── main.go                 服务入口（内嵌控制台静态资源、路由装配）
├── config.json             服务端配置（端口 / 数据目录 / 浏览器 / AI）
├── runtime/uitest-keys.js  ★ 浏览器侧共享运行时：稳定 ID 绑定 + 多级兜底 + 自愈定位
├── internal/
│   ├── model/              数据模型（与 docs/API.md 严格一致）
│   ├── store/              数据存储（单文件 JSON，原子落盘，零外部依赖）
│   ├── ai/                 AI 数据生成（OpenAI 兼容接口 + 内置规则兜底）
│   ├── cdp/                精简 CDP 客户端（WebSocket）
│   ├── browser/            内置浏览器生命周期（启动 / 连接 / 标签页 / 截图 / 上传文件）
│   ├── engine/             执行引擎（变量替换 + 定位 + 自愈 + 断言 + 逐步日志）
│   └── api/                REST + SSE 接口
├── web/                    控制台前端（原生 JS，无需构建）
├── extension/              浏览器插件（MV3 录制端）
├── demo/                   示例被测页面（演练用，无真实后端）
├── scripts/smoke.ps1       端到端自检（造录制 → 生成数据 → 真跑 → 打印结果）
└── docs/API.md             前后端唯一契约
```

---

## 一、快速开始

### 1. 编译并启动服务端

```powershell
cd uitest
go build -o uitest-server.exe .
.\uitest-server.exe          # 默认 http://127.0.0.1:8490
```

或直接用脚本（编译 + 后台启动 + 健康检查）：

```powershell
powershell -ExecutionPolicy Bypass -File .\start.ps1     # 启动（-Port 8491 可换端口）
powershell -ExecutionPolicy Bypass -File .\stop.ps1      # 停止
```

> 系统默认禁止运行未签名脚本时，必须带 `-ExecutionPolicy Bypass`（或先 `Set-ExecutionPolicy -Scope Process Bypass`）。

打开 <http://127.0.0.1:8490/> 就是控制台。想先看效果，可以打开示例被测页 <http://127.0.0.1:8490/demo/>。

### 2. 装插件（录制端）

1. 打开 `edge://extensions`（或 `chrome://extensions`）→ 打开「开发人员模式」
2. 「加载解压缩的扩展」→ 选择本项目的 `extension` 目录
3. 点插件图标 → 填服务端地址（默认 `http://127.0.0.1:8490`）→「测试连接」应显示绿点
4. **装/更新插件后要刷新一次被测页面**（内容脚本只在页面加载时注入）

> `extension/uitest-keys.js` 是 `runtime/uitest-keys.js` 的副本（插件与服务端必须用同一份定位算法）。
> 修改运行时后执行：`Copy-Item runtime\uitest-keys.js extension\uitest-keys.js -Force`

### 3. 录制

在插件弹窗里填**录制名称**（例如「新建服务商品」「上传商品」「设置秒杀活动」）→「开始录制」→ 在被测页面正常操作：

- 点击按钮 / 链接 → `点击【保存商品】`
- 输入框失焦 → `在【商品名称】输入内容`（值以 `{{product_name}}` 形式保存，执行时换成数据）
- 下拉 / 勾选 / 文件上传 / 页面跳转 → 各自生成步骤
- 每一步都自动生成中文备注；弹窗里「上一步备注」可以随时补一句人话（**备注越清楚，回放越不容易错位**）

停止录制后，控制台 `#/recordings` 就能看到这条录制和它的全部步骤。

### 4. 生成数据 + 执行

在录制详情页：

1. 右侧「数据准备指令」写一句中文，例如：`商品名称带时间戳，价格 99-299，库存 50，分类选"服务类"`
2. 点「AI 生成测试数据」→ 得到一份 key / 中文名 / 值 / 生成理由 的表格，可逐项手改
3. 点「执行测试」→ 服务端用内置浏览器真跑，弹出 `#/runs/{id}` 运行详情

### 5. 看全过程

运行详情页有：数据准备结果、**逐步结果**（状态 / 实际使用的定位 / 是否自愈 / 耗时 / 失败原因 / 失败截图）、**实时日志**（SSE 推送，跟着滚）。

---

## 二、核心设计：元素怎么定位才不会"下次就变了"

这是自动化最容易烂掉的地方，本项目有三层机制：

### 第 1 层：稳定 ID 绑定（`data-uitest-id`）

`runtime/uitest-keys.js` 给每个可交互元素算一个**确定性**的键：

```
ut_<fnv1a32( 标签名 | type | id | name | aria-label | placeholder | 业务标签文字 )>
```

- 只取业务语义，**不含** DOM 层级、随机数、时间戳 → 页面刷新、重新录制、服务端回放算出来的键**完全一致**；
- 插件录制时算一次并写进 `locator.strategy = "uid"`，服务端执行前用**同一份代码**重算，`uid` 定位永远命中；
- 控制台里 `uid=` 会显示成绿色「ID 绑定」徽章，元素库页可以查这些键的命中次数。

> 如果你们的页面能直接加 `data-testid` / 有意义的 `id`，那更稳——定位优先级里 `id` / `name` 同样排在 CSS 之前。

### 第 2 层：多级兜底

每个步骤同时保存：`uid` → `id` → `name` → `css` → `text` / `label` / `placeholder`。
主定位失效会自动依次尝试下一级，日志里会写明实际用了哪一条。

### 第 3 层：自愈（self-healing）+ 回写

全部兜底都失效时，用录制时保存的**元素指纹**（标签、type、业务标签文字、placeholder、可见文本、aria、下拉候选）在页面上做相似度打分，
只有"唯一最高分且达到阈值"才认为命中，并在日志里打上橙色「自愈」徽章。

运行详情页右上角的「把自愈定位写回录制」会把这些新键写回步骤（旧定位保留为备用），
下次执行就恢复成毫秒级命中——**录制会自己越跑越稳**。

---

## 三、AI 数据准备

两种模式，配置在控制台「设置」页（也支持环境变量）：

| 模式 | 触发条件 | 说明 |
|---|---|---|
| `ai` | 填了 `base_url` + `api_key` + `model` | 调 OpenAI 兼容的 `/chat/completions`，按字段语义 + 你的中文指令生成合法数据，返回 `values` 与 `notes`（生成理由） |
| `rules` | 未配置或调用失败 | 内置规则：名称带时间戳、价格区间、库存整数、手机号、邮箱、未来日期、下拉从录制时采集的候选项里挑…；能读懂「价格 99-299」「库存 50」这类指令 |

生成字段来自录制里的输入类步骤（`input` / `select` / `upload` / `check`），字段名叫 `data_key`（插件自动从 label/name/placeholder 推导 snake_case，可在控制台改）。

执行时步骤里的 `{{key}}` 会被替换成数据值，另外内置 `{{now}}` `{{today}}` `{{datetime}}` `{{timestamp}}` `{{uuid}}` `{{random:8}}`。

环境变量：`UITEST_AI_BASE_URL`、`UITEST_AI_KEY`、`UITEST_AI_MODEL`。

---

## 四、内置浏览器

默认做法：服务端按需拉起一个**独立用户目录**的 Edge/Chrome（`data/browser-profile`），
调试端口自动选空闲端口，然后通过 CDP 建标签页、执行、截图。

```jsonc
// config.json
"browser": {
  "executable": "",        // 留空=自动探测（Edge 优先）
  "headless": false,       // true=无头
  "step_delay_ms": 260,    // 每步之间等多久（给前端渲染留时间）
  "action_timeout_ms": 15000,
  "no_sandbox": false,
  "debug_url": ""          // 见下：连接已有浏览器
}
```

### 方式二：连接一个已经开着的浏览器

适合 CI / 容器 / 受限环境，或者想复用你已登录的浏览器：

```powershell
# 手工起一个带调试端口的 Edge
& "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" `
  --remote-debugging-port=9400 --user-data-dir="$env:TEMP\edge-uitest" about:blank
```

然后在控制台「设置 → 内置浏览器」把 `debug_url` 填成 `http://127.0.0.1:9400`。
此后服务端不再自己启动浏览器，只连它（关闭时也只断开，不杀进程）。

### 排错：`Target crashed` / `标签页不可用`

说明浏览器进程起来了，但它的渲染进程被运行环境杀掉或起不来（常见于受限的 Job 对象、容器、被杀软拦截）。
按顺序试：

1. 设置里打开 `no_sandbox`（启动时追加 `--no-sandbox`）；
2. 改用上面的「连接已有浏览器」（`debug_url`）；
3. 用 `--headless` 跑；
4. 看 `data/logs/browser.log` 里的原始报错。

---

## 五、自检（不装插件也能验证整条链路）

```powershell
powershell -ExecutionPolicy Bypass -File scripts\smoke.ps1
```

它会：建一条「新建服务商品」录制（10 步，其中第 2 步故意用**失效 uid** 验证自愈）
→ 调 AI/规则生成数据 → 用内置浏览器真跑 → 打印逐步结果 → 把自愈定位回写 → 打印控制台链接。

通过时的输出形如：

```
#2  click  点击【新建商品】按钮（故意用失效 uid，验证自愈）  passed  [自愈] 3ms 点击 新建商品
#3  input  在【商品名称】输入内容                      passed   68ms 输入「商品-自动化测-1021205906」到 商品名称
...
执行结果：passed
```

---

## 六、接口

全部接口见 **[docs/API.md](docs/API.md)**（字段以它为准，插件与控制台都按它实现）。常用几条：

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/recordings` | 新建录制（插件「开始录制」调这个） |
| POST | `/api/recordings/{id}/steps` | 追加一个步骤 |
| POST | `/api/ai/dataset` | 生成测试数据 |
| POST | `/api/runs` | 执行一次（数据缺省时自动生成） |
| GET | `/api/runs/{id}` | 运行详情（逐步结果 + 日志） |
| GET | `/api/runs/{id}/events` | SSE 实时日志 |
| POST | `/api/runs/{id}/heal` | 把自愈定位写回录制 |
| POST | `/api/elements/bind` | 登记元素绑定（插件上报） |

---

## 七、已知限制

1. 插件内容脚本只在页面加载时注入 —— 装/更新插件后需要刷新被测页面。
2. iframe 里的操作会记录 `locator.frame_url`，但当前执行器以主框架为主，跨 iframe 步骤建议人工确认。
3. 富文本编辑器（Quill / ProseMirror 等 contenteditable）可能不触发原生 `input` 事件，取值可能不准。
4. 文件上传：插件只能录到文件名；执行时用数据准备里的值当本地路径，没有就用内置生成的一张示例 PNG。
5. 纯中文 `label`/`name` 推导出来的 `data_key` 会退化成 `field_N`，建议在控制台改成有意义的名字。
6. 运行记录默认保留最近 200 条（`config.json` 的 `storage.keep_runs`）。
7. 单机单进程设计：不做多租户、不做并发编排（可并行跑多个 run，但共享一个浏览器进程）。

---

## 八、环境变量

| 变量 | 作用 |
|---|---|
| `UITEST_PORT` | 监听端口（默认 8490） |
| `UITEST_HOST` | 监听地址（默认 127.0.0.1） |
| `UITEST_DATA_DIR` | 数据目录（默认 `data`） |
| `UITEST_BROWSER` | 浏览器可执行文件路径 |
| `UITEST_HEADLESS` | `1` = 无头 |
| `UITEST_AI_BASE_URL` / `UITEST_AI_KEY` / `UITEST_AI_MODEL` | AI 数据生成配置 |
