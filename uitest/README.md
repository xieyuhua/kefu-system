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
├── scripts/fetch-browser.ps1  下载一份独立内核（Chrome for Testing）到 browser/
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

在录制详情页（「数据准备」面板**默认就是悬浮窗**，可拖动、可缩放：

- 按住面板标题栏拖动，右下角拉伸改尺寸，位置/尺寸会记住；
- 「收起」只留标题栏，再点「展开」恢复；双击标题栏也能收起/展开；
- 只有悬浮一种形态（没有"停靠"），位置/尺寸记在 localStorage：`uitest.pos.data`）：

1. 「数据准备指令」写一句中文，例如：`商品名称带时间戳，价格 99-299，库存 50，分类选"服务类"`
2. 点「AI 生成测试数据」→ 得到一份 key / 中文名 / 值 / 生成理由 的表格，可逐项手改
3. 点「执行测试」→ 服务端用内置浏览器真跑，弹出 `#/runs/{id}` 运行详情

### 5. 看全过程

运行详情页有：数据准备结果、**逐步结果**（状态 / 实际使用的定位 / 是否自愈 / 耗时 / 失败原因 / 失败截图）、**实时日志**（SSE 推送，跟着滚）。

**实时日志默认就是悬浮窗**（右下角，可拖可缩放）：**按住标题栏拖动**，右下角拉伸改尺寸，
「收起」只留标题栏、再点「展开」恢复；位置/尺寸记在 localStorage（`uitest.logPos`），双击标题栏也能收起/展开。
面板只有悬浮一种形态，不存在"停靠"。

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

### Vue / React 项目重新打包后怎么定位（2.0 重建安全）

Vue/React 每次打包都会变一批"构建产物"，这些**一律不参与定位**：

| 会变的东西 | 例子 | 处理 |
|---|---|---|
| CSS Modules / scoped hash 类名 | `.product-card_1a2b3c`、`.css-9f8e7d`、`.sc-bdVaJa` | 完全不进选择器（css 只用 `tag:nth-of-type` 结构） |
| 作用域属性 | `data-v-7ba5bd90` | 忽略 |
| 组件库自动 id | `el-id-3812-12`、`:r1:`、`input-3`、`rc_select_7` | 命中"不稳定 token"规则就跳过，不写进 key、不当 id 定位 |

只认业务语义。定位链（从最稳到最不稳）：

```
uid（data-uitest-id，确定性算法：tag|type|稳定id|name|aria|placeholder|label|表单项标题|表格列）
  → attr（data-testid / data-test / data-cy 等你们自己写的稳定属性）
  → id / name（仅当"看起来稳定"）
  → label（表单项标题文字）→ near（标题 → 同一表单项里的控件）
  → role（ARIA 角色 + 按钮文字）→ text
  → cell（表格行文字 + 按钮文字，行内操作神器）
  → placeholder → css（纯结构）→ xpath
全失败 → 元素指纹自愈打分（表单项标题/列名/name/placeholder 都参与打分），唯一最高分才认
```

几个关键工程细节（都是踩坑后改的）：

- **表单项标题只认最近的精确容器**：`formItemOf` 先试 `.el-form-item / .ant-form-item …`，
  再退到模糊类名，否则 `.el-form-item__content` 会抢先命中，导致三个输入框都叫「商品名称」。
- **`near` 取最内层容器**：祖先容器（`#app`、`body`、`.el-form`）里也含标题文字，
  按文档顺序取第一个会"填价格填到商品名称里"，所以按"后代元素最少"选。
- **按钮不走 label/near**：按钮文字会被误当成表单项标题、抓到旁边输入框；
  按钮只走 `cell` / `role` / `text`。
- **表格行指纹用行首单元格文字**，不用整行文本（改版加一列就变）。
- **同签名元素自动加 `_2/_3` 后缀**去重，保证 key 在本页唯一。

验证方式（`demo/vue-v1.html` / `demo/vue-v2.html` 是刻意造的两个"打包版本"）：
在 A 版录制 → 到 B 版（hash 全变、id 全变、多一层包装 div、表格多一列、行内按钮顺序调换）回放，
实测 **6/6 全部由 uid 直接命中**。控制台里每个定位还会显示 **稳 / 中 / 弱** 徽章，
弱定位（css/xpath/text）建议跑一次「把自愈定位写回录制」沉淀成 uid。

### 三项「辅助功能」（设置页可逐项开关）

| 开关 | 做什么 | 关掉会怎样 |
|---|---|---|
| **中文标题自动转字段名** | 录制时用业务词典把中文标签转成英文字段名：`记住密码` → `remember_password`、`商品名称` → `product_name`、`价格（元）` → `price`。只给"要填数据"的动作生成，按钮/断言不给 | 纯中文标签会退化成 `field_25` 这种键，AI 生成数据与日志都看不懂 |
| **缺 data-testid 的元素标红** | 录制详情里把没有 `data-testid / data-test / data-cy` 一类稳定锚点的步骤打「缺 testid」红标，并在顶部汇总列出是哪几个元素，方便直接推给前端 | 不显示提醒，只能自己看定位策略 |
| **执行时自动刷新过期的 uid** | 某步的 uid 过期、但靠兜底策略（表单项标题 / 按钮文字 / 表格行）命中了元素时，自动把新 uid 写回录制 | 只在日志里提示"uid 已过期"，不落盘 |

三个开关都通过 `PUT /api/settings` 的 `assist` 段持久化，控制台从 `/api/health` 的 `assist` 段拿到当前值。

### 一键刷新定位（老录制升级）

录制详情右上角「**一键刷新定位**」= `POST /api/recordings/{id}/rebind`：
打开起始页 → 逐步重新定位 → 把元素当前的 `data-uitest-id` 写回录制（旧定位降级为兜底），
顺便按业务词典修正 `data_key`。

注意它是**语义优先**的：老录制的 `css=input.el-input__inner` 这种定位本身有歧义（三个输入框都会命中第一个），
所以刷新时会先用「表单项标题 → 同一表单项里的控件」重新定位，失败才退回原定位，避免把错误固化。

实测：一条 css 定位 + `field_25` 的老录制，刷新后 5 个定位全部升级成 `uid`，
把起始页换成另一个打包版本（hash/id/包装层全变）后执行 **7/7 通过**。

### 数据快照（误删可回退）

服务端每次启动都会把 `data/uitest.json` 快照到 `data/backups/`（保留最近 5 份）。
设置页「数据快照」列出每份的时间/内容条数/大小，点「恢复」即 `POST /api/backups/{name}/restore`。
恢复会用快照覆盖当前数据，**覆盖前会先把当前状态再存一份快照**，所以误恢复也能退回。

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
  "no_sandbox": false,     // 渲染进程被杀时打开（下面「启动自愈」通常已自动处理）
  "debug_url": "",         // 非空=连接已有浏览器，不再自己拉起
  "kernel_dir": "",        // 自带内核目录；留空用自己的 <程序目录>/browser
  "extra_args": []         // 额外启动参数（高级）
  // resolved_flags 由服务端写回：它自动试出来的可用参数组合，下次直接复用
}
```

### 启动自愈：为什么"浏览器打得开、但打不开网页"

受限环境（容器 / 受限 Job 对象 / 安全软件拦截）里，Chromium 的**渲染进程**会因为沙箱注册失败
直接崩掉：窗口能出来，但任何网址都是白屏，CDP 报 `Target crashed`，日志里是
`AppContainerRegisterSid failed` / `Failed to get peer PID`。

服务端不会就这么把锅甩给你，`Ensure()` 里做了三件事：

1. **逐组试参数**：默认 → `--no-sandbox` → `--no-sandbox --in-process-gpu --disable-gpu` …，
   每启动一次都**真实打开一个页面并读 `document.title`** 来判定渲染进程是否活着；
2. **记住能用的组合**（`resolved_flags` 落盘），下次启动直接用，不再试错；
3. **接管/清理遗留实例**：server 重启后如果旧浏览器还活着，用 `<profile>/DevToolsActivePort` 接管；
   接管到但渲染已坏的，先 `Browser.close` 再重启；重试换独立 profile 目录，避免 profile 被占用；
   成功的那套「参数 + profile 目录」会固化到设置里，下次直接复用（登录态也因此保留）。

> 时间预期：正常机器首次自适应约 20s，之后启动 1s 内；如果浏览器进程被**强杀**（例如直接 taskkill 服务端），
> 那个 profile 目录要重新初始化，第一次启动会再花 10~40s，之后就快了。

控制台「设置 → 诊断浏览器」会把这些一次性摊出来：内核路径、启动参数、页面标题、readyState、
诊断结论、浏览器日志尾部。**"浏览器页面不支持 / 打不开网站"先点这个按钮。**

### 用自带内核（不依赖系统浏览器）

```powershell
powershell -ExecutionPolicy Bypass -File scripts\fetch-browser.ps1            # 官方源
powershell -ExecutionPolicy Bypass -File scripts\fetch-browser.ps1 -Mirror npmmirror   # 国内镜像
```

脚本会把 Chrome for Testing 解压到 `<项目>/browser/chrome-win64/chrome.exe`，
服务端默认就优先用它（设置里 `kernel_dir` 留空即可）；也可以手工解压任意 Chromium 内核，
然后在控制台「设置 → 自带内核目录」填那个目录。

> 注意：自带内核是**全新用户目录**（`data/browser-profile`），不带你系统浏览器里的登录态、
> 书签和插件。要复用已登录的会话，请用下面的「连接已有浏览器」。

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
8. 内置浏览器只能访问**这台机器能访问的网络**：离线/内网环境里打开 baidu 这类外网站点必然失败，
   这是网络限制不是浏览器问题；内网/本机地址（如 `http://127.0.0.1:8480/admin/`）正常。

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
