# UI 自动化测试平台 —— API 契约（唯一标准）

服务端：`uitest-server`（Go，默认 `http://127.0.0.1:8490`）
客户端：浏览器插件（MV3，录制端）+ Golang 服务自带网页控制台（查看端）
所有接口前缀 `/api`，请求/响应均为 `application/json; charset=utf-8`，字段名统一 `snake_case`。

通用响应：成功直接返回对象/数组；失败返回 `{"error":"中文错误信息"}` 且 HTTP 4xx/5xx。
CORS：`/api/**` 允许任意来源（插件页面来源为 `chrome-extension://...`）。

---

## 1. 数据模型

### 1.1 Locator（元素定位）

```json
{
  "strategy": "uid",                  // uid | id | name | css | xpath | text | label | placeholder
  "value": "ut_7f3a91c2",             // uid = data-uitest-id 属性值（首选，最稳定）
  "fallbacks": [                       // 备用定位，按顺序尝试
    {"strategy": "css", "value": "#product-name"},
    {"strategy": "name", "value": "name"},
    {"strategy": "text", "value": "新建商品"}
  ],
  "frame_url": ""                      // 预留：iframe 内页面地址
}
```

**定位优先级**：`uid` > `id` > `name` > `css` > `xpath` > `label`/`text`/`placeholder`；
全部失败时执行**自愈定位**（用 `target` 指纹做相似度匹配），自愈命中会在运行日志中标记
`healed=true`，并可通过 `POST /api/runs/{id}/heal` 把新定位回写到录制。

### 1.2 Target（元素指纹，用于自愈与 AI 理解）

```json
{
  "tag": "input",
  "type": "text",
  "label": "商品名称",            // 关联 label / 就近文本 / 表单项标题
  "text": "",                     // 元素自身可见文本（按钮类）
  "placeholder": "请输入商品名称",
  "aria": "",
  "role": "",
  "href": "",
  "page": "/admin/#/products",    // 录制时所在页面（path + hash）
  "field_kind": "text",           // text|number|price|date|datetime|email|phone|url|textarea|select|checkbox|file|password|other
  "options": ["服务类", "实物类"]  // 下拉框候选文本（录制时采集，供 AI 生成"一定能选中"的值）
}
```

### 1.3 Step（一个操作步骤）

```json
{
  "id": "stp_9f2c1b",
  "recording_id": "rec_ab12",
  "index": 1,
  "action": "click",              // navigate|click|input|select|check|uncheck|key|hover|wait|upload|assert|script
  "annotation": "点击【新建商品】按钮",   // 人可读备注，录制时自动生成，可被人工/AI 修正
  "locator": { ... },
  "target": { ... },
  "data_key": "",                 // 输入类步骤的业务字段名，如 product_name（AI 按此生成数据）
  "value": "{{product_name}}",    // 期望输入的文本；支持 {{变量}}
  "expect": "",                   // 断言：普通文本=页面需出现的文字；url:xxx=URL 需包含；js:表达式=需为真
  "wait_ms": 0,                   // 执行后额外等待毫秒
  "optional": false,              // true=失败不中断
  "note": "",                     // 补充说明（会更少误差：备注+定位一起用）
  "created_at": "2026-02-01T10:00:00+08:00"
}
```

`value` 支持的变量：`{{字段key}}`、`{{now}}`、`{{today}}`、`{{datetime}}`、`{{timestamp}}`、`{{uuid}}`、`{{random:8}}`。

### 1.4 Recording（一条录制 = 一个业务场景）

```json
{
  "id": "rec_ab12",
  "name": "新建服务商品",
  "description": "在后台商品页新建一个服务类商品并保存",
  "start_url": "http://127.0.0.1:8480/admin/#/products",
  "tags": ["商品", "回归"],
  "status": "draft",              // draft | ready
  "step_count": 6,
  "created_at": "...", "updated_at": "...",
  "steps": [ {Step}, ... ]        // 仅详情接口返回
}
```

### 1.5 Element（元素库：稳定 ID 绑定表）

```json
{
  "key": "ut_7f3a91c2",           // data-uitest-id
  "page": "/admin/#/products",
  "selector": "input[name=name]",
  "descriptor": "商品名称 输入框",
  "tag": "input", "type": "text", "label": "商品名称",
  "hits": 3, "last_seen": "...", "broken": false
}
```

### 1.6 Run（一次任务执行）

```json
{
  "id": "run_c81d",
  "recording_id": "rec_ab12",
  "recording_name": "新建服务商品",
  "status": "queued",             // queued | running | passed | failed | canceled
  "mode": "server",               // server = 服务端内置浏览器执行
  "dataset": {"product_name": "自动化测试商品-9382", "price": "199.00"},
  "dataset_source": "ai",         // ai | rules | manual
  "dataset_fields": [ {"key":"product_name","label":"商品名称","kind":"text","note":"AI：符合 2-20 字"} ],
  "total_steps": 10, "passed_steps": 10,   // 列表页摘要用
  "step_results": [
    {
      "index": 1, "step_id": "stp_9f2c1b", "action": "click",
      "annotation": "点击【新建商品】按钮",
      "status": "passed",           // passed | failed | skipped
      "value_used": "", "locator_used": "uid=ut_7f3a91c2",
      "healed": false, "message": "", "duration_ms": 132,
      "screenshot": "data/runs/run_c81d/step_01.png"
    }
  ],
  "logs": [ {"ts":"...","level":"info","index":1,"message":"点击 新建商品"} ],
  "started_at": "...", "finished_at": "...", "error": ""
}
```

---

## 2. 接口清单

### 2.1 录制（Recordings）

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| GET | `/api/recordings?q=&tag=` | – | `{"items":[Recording 摘要（无 steps）]}` |
| POST | `/api/recordings` | `{name, description?, start_url?, tags?}` | Recording |
| GET | `/api/recordings/{id}` | – | Recording（含 steps） |
| PATCH | `/api/recordings/{id}` | `{name?,description?,tags?,status?}` | Recording |
| DELETE | `/api/recordings/{id}` | – | `{"ok":true}` |
| POST | `/api/recordings/{id}/steps` | Step 字段（除 id/recording_id/index） | Step |
| PATCH | `/api/recordings/{id}/steps/{step_id}` | `{annotation?,value?,data_key?,expect?,locator?,target?,wait_ms?,optional?,note?,action?}` | Step |
| DELETE | `/api/recordings/{id}/steps/{step_id}` | – | `{"ok":true}` |
| POST | `/api/recordings/{id}/reorder` | `{"step_ids":["stp_a","stp_b"]}` | Recording |
| POST | `/api/recordings/{id}/duplicate` | `{name?}` | Recording |
| GET | `/api/recordings/{id}/export` | – | Recording（含 steps，可直接 import） |
| POST | `/api/recordings/import` | Recording JSON | Recording |

### 2.2 元素库（Elements）

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| POST | `/api/elements/bind` | `{page,key?,selector,tag?,type?,label?,descriptor?}` | Element（key 缺省时服务端生成 `ut_xxxxxxxx`） |
| GET | `/api/elements` | – | `{"items":[Element]}` |
| DELETE | `/api/elements/{key}` | – | `{"ok":true}` |

### 2.3 AI 数据准备（AI）

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| POST | `/api/ai/dataset` | `{recording_id, instruction?, count?}` | `{"fields":[Field],"dataset":{},"notes":{},"source":"ai|rules","warning":""}` |

Field：`{"key","label","kind","value","note"}`。
`source=rules` 表示未配置大模型或调用失败时的内置规则生成（保证离线可用）。

### 2.4 执行（Runs）

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| POST | `/api/runs` | `{recording_id, dataset?, dataset_source?, ai?:{enabled,instruction}, mode?:"server", headless?:bool}` | Run（`status=queued`，异步执行） |
| GET | `/api/runs?recording_id=&limit=` | – | `{"items":[Run 摘要（无 logs/step_results）]}` |
| GET | `/api/runs/{id}` | – | Run（全量） |
| POST | `/api/runs/{id}/cancel` | – | Run |
| POST | `/api/runs/{id}/heal` | – | `{"updated":2,"steps":[...]}`（把自愈定位写回录制） |
| GET | `/api/runs/{id}/events` | – | SSE：`data: {"type":"log|step|status","...":...}`，结束发 `data: {"type":"end"}` |

### 2.5 内置浏览器（Browser）

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| GET | `/api/browser/status` | – | `{running,executable,debug_url,profile_dir,version}` |
| POST | `/api/browser/launch` | `{url?}` | `{ok,debug_url}` |
| POST | `/api/browser/close` | – | `{ok}` |

### 2.6 设置 / 元信息

| 方法 | 路径 | 入参 | 返回 |
|---|---|---|---|
| GET | `/api/settings` | – | `{ai:{base_url,model,has_key,key_hint,timeout_sec},browser:{executable,headless,step_delay_ms,action_timeout_ms,no_sandbox,debug_url},server:{port,version}}` |
| PUT | `/api/settings` | 同上（`ai.api_key` 只写不读） | 同上 |
| GET | `/api/health` | – | `{ok:true,version,recordings,runs,browser_running}` |

---

## 3. 插件 → 服务端 调用顺序（录制流程）

1. 弹窗里填「录制名称」→ `POST /api/recordings` → 拿到 `recording_id`（存 `chrome.storage.local`）
2. 用户开始操作页面，每个动作：`POST /api/recordings/{id}/steps`（插件同时上报 `locator`/`target`/`annotation`/`data_key`）
3. 顺手登记元素：`POST /api/elements/bind`（key = data-uitest-id）
4. 点击「停止」→ `POST /api/recordings/{id}`（`{"status":"draft"}`）或直接结束；控制台可见

## 4. 执行流程（服务端）

`POST /api/runs` → 服务端：AI/规则生成数据 → 启动/复用内置浏览器 → 逐步执行（变量替换 + 多级定位 + 自愈）→
逐步写 `step_results`/`logs`（SSE 实时推送）→ 结束置 `passed/failed`。控制台 `#/runs/{id}` 看全过程细节。

---

## 5. 补充说明

### 5.1 文件上传

`action="upload"` 的步骤：服务端先用定位器找到元素，再用 CDP `DOM.setFileInputFiles` 注入本地文件。
文件路径取步骤 `value` 里替换后的值；该路径不存在时，服务端会生成一张示例 PNG（`data/tmp/auto-test-upload.png`）用于跑通流程。

### 5.2 自愈与回写

`step_results[].healed = true` 表示该步主定位/兜底全部失效、由元素指纹相似度匹配命中，
此时 `locator_used` 形如 `uid=ut_xxx (自愈:标签一致、文本一致)`，`heal_from` 记录失效前的定位链。
调用 `POST /api/runs/{id}/heal` 会把这些新 `uid` 写回录制步骤（旧定位降级为 `fallbacks[0]`），返回 `{updated, steps:[{index,from,to}]}`。

### 5.3 浏览器两种模式

- **自己拉起**（默认）：`browser.debug_url` 为空，服务端启动 Edge/Chrome（独立 user-data-dir）；
- **连接已有**：`browser.debug_url = "http://127.0.0.1:9400"`，服务端只连接该 CDP 端点，关闭时只断开连接。

标签页命令走**页面级端点** `ws://127.0.0.1:<port>/devtools/page/<targetId>`
（新版 Chromium 的浏览器级扁平化会话不返回会话命令响应，已实测不适用）。