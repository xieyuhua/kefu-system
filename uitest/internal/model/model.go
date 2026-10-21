// Package model 定义平台的全部数据结构。
// 字段名与 docs/API.md 的 JSON 契约严格一致（snake_case），前端与插件都按此解析。
package model

import "time"

// Locator 元素定位描述。strategy 取值：
// uid(data-uitest-id，首选) | id | name | css | xpath | text | label | placeholder
type Locator struct {
	Strategy  string    `json:"strategy"`
	Value     string    `json:"value"`
	Fallbacks []Locator `json:"fallbacks,omitempty"`
	FrameURL  string    `json:"frame_url,omitempty"`
}

// Target 元素指纹：录制时保存，执行时用于自愈定位与 AI 理解字段语义。
type Target struct {
	Tag         string `json:"tag"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Text        string `json:"text"`
	Placeholder string `json:"placeholder"`
	Aria        string `json:"aria"`
	Role        string `json:"role"`
	Href        string `json:"href"`
	Page        string `json:"page"`
	FieldKind   string `json:"field_kind"`
	// Options 下拉框候选文本（录制时采集，供 AI 生成"一定能选中的值"）
	Options []string `json:"options,omitempty"`
}

// Describe 返回人可读的目标描述（用于日志与默认备注）。
func (t Target) Describe() string {
	for _, s := range []string{t.Label, t.Text, t.Placeholder, t.Aria, t.Tag} {
		if s != "" {
			return s
		}
	}
	return "元素"
}

// Step 一个操作步骤。
type Step struct {
	ID          string    `json:"id"`
	RecordingID string    `json:"recording_id,omitempty"`
	Index       int       `json:"index"`
	Action      string    `json:"action"`
	Annotation  string    `json:"annotation"`
	Locator     Locator   `json:"locator"`
	Target      Target    `json:"target"`
	DataKey     string    `json:"data_key,omitempty"`
	Value       string    `json:"value,omitempty"`
	Expect      string    `json:"expect,omitempty"`
	WaitMS      int       `json:"wait_ms,omitempty"`
	Optional    bool      `json:"optional,omitempty"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Recording 一条录制 = 一个业务场景（新建服务商品 / 上传商品 / 设置活动 …）。
type Recording struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	StartURL    string    `json:"start_url"`
	Tags        []string  `json:"tags"`
	Status      string    `json:"status"` // draft | ready
	StepCount   int       `json:"step_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Steps       []Step    `json:"steps,omitempty"`
}

// Summary 列表用副本（不含 steps，保留 step_count）。
func (r Recording) Summary() Recording {
	count := len(r.Steps)
	if r.StepCount > 0 {
		count = r.StepCount
	}
	r.Steps = nil
	r.StepCount = count
	return r
}

// Clone 深拷贝，避免调用方与存储共享底层切片。
func (r Recording) Clone() Recording {
	c := r
	c.Tags = append([]string(nil), r.Tags...)
	if r.Steps != nil {
		c.Steps = make([]Step, len(r.Steps))
		copy(c.Steps, r.Steps)
		for i := range c.Steps {
			c.Steps[i].Locator.Fallbacks = append([]Locator(nil), r.Steps[i].Locator.Fallbacks...)
		}
	}
	return c
}

// StepResult 单个步骤的执行结果。
type StepResult struct {
	Index       int    `json:"index"`
	StepID      string `json:"step_id"`
	Action      string `json:"action"`
	Annotation  string `json:"annotation"`
	Status      string `json:"status"` // passed | failed | skipped
	ValueUsed   string `json:"value_used,omitempty"`
	LocatorUsed string `json:"locator_used,omitempty"`
	Healed      bool   `json:"healed"`
	HealFrom    string `json:"heal_from,omitempty"` // 自愈前失效的定位
	Message     string `json:"message,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	Screenshot  string `json:"screenshot,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
}

// LogEntry 执行日志一行。
type LogEntry struct {
	TS      time.Time `json:"ts"`
	Level   string    `json:"level"` // info | warn | error
	Index   int       `json:"index"` // 关联步骤序号，0 表示整体
	Message string    `json:"message"`
}

// Run 一次任务执行。
type Run struct {
	ID            string            `json:"id"`
	RecordingID   string            `json:"recording_id"`
	RecordingName string            `json:"recording_name"`
	Status        string            `json:"status"` // queued | running | passed | failed | canceled
	Mode          string            `json:"mode"`   // server
	Dataset       map[string]string `json:"dataset"`
	DatasetSource string            `json:"dataset_source"` // ai | rules | manual
	DatasetFields []AIField         `json:"dataset_fields,omitempty"`
	DatasetNotes  map[string]string `json:"dataset_notes,omitempty"`
	StepResults   []StepResult      `json:"step_results"`
	Logs          []LogEntry        `json:"logs"`
	StartedAt     time.Time         `json:"started_at,omitempty"`
	FinishedAt    time.Time         `json:"finished_at,omitempty"`
	Error         string            `json:"error,omitempty"`
	Healables     int               `json:"healables"`
	// TotalSteps / PassedSteps 给列表页用的摘要统计
	TotalSteps  int `json:"total_steps"`
	PassedSteps int `json:"passed_steps"`
}

// Summary 列表用副本（不含 logs / step_results）。
func (r Run) Summary() Run {
	r.Logs = nil
	r.StepResults = nil
	r.Dataset = nil
	r.DatasetFields = nil
	return r
}

// Clone 深拷贝。
func (r Run) Clone() Run {
	c := r
	if r.Dataset != nil {
		c.Dataset = make(map[string]string, len(r.Dataset))
		for k, v := range r.Dataset {
			c.Dataset[k] = v
		}
	}
	if r.DatasetNotes != nil {
		c.DatasetNotes = make(map[string]string, len(r.DatasetNotes))
		for k, v := range r.DatasetNotes {
			c.DatasetNotes[k] = v
		}
	}
	c.DatasetFields = append([]AIField(nil), r.DatasetFields...)
	c.StepResults = append([]StepResult(nil), r.StepResults...)
	c.Logs = append([]LogEntry(nil), r.Logs...)
	return c
}

// AIField 一条测试数据字段定义（AI 或规则生成）。
type AIField struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Kind  string   `json:"kind"`
	Value string   `json:"value"`
	Note  string   `json:"note,omitempty"`
	Enum  []string `json:"enum,omitempty"`
}

// Element 元素库条目：data-uitest-id 与页面语义的绑定索引。
type Element struct {
	Key        string    `json:"key"`
	Page       string    `json:"page"`
	Selector   string    `json:"selector"`
	Descriptor string    `json:"descriptor"`
	Tag        string    `json:"tag"`
	Type       string    `json:"type"`
	Label      string    `json:"label"`
	Hits       int       `json:"hits"`
	LastSeen   time.Time `json:"last_seen"`
	Broken     bool      `json:"broken"`
}

// Settings 运行时设置（控制台可改，落盘持久化）。
type Settings struct {
	AI      AISettings      `json:"ai"`
	Browser BrowserSettings `json:"browser"`
}

// AISettings 大模型接入配置（OpenAI 兼容）。
type AISettings struct {
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
}

// BrowserSettings 内置浏览器配置。
type BrowserSettings struct {
	Executable      string `json:"executable"`
	Headless        bool   `json:"headless"`
	StepDelayMS     int    `json:"step_delay_ms"`
	ActionTimeoutMS int    `json:"action_timeout_ms"`
	// NoSandbox 某些受限/虚拟化环境里 Chromium 渲染进程会因沙箱注册失败而崩溃
	// （CDP 报 Target crashed），此时打开它即追加 --no-sandbox。
	NoSandbox bool `json:"no_sandbox"`
	// DebugURL 非空时不自己拉起浏览器，而是连接这个已存在的 CDP 端点
	// （例如先手工执行 msedge --remote-debugging-port=9400，再填 http://127.0.0.1:9400）。
	// 适用于 CI / 容器 / 受限环境，或调试时复用你已登录的浏览器。
	DebugURL string `json:"debug_url"`
}

// DefaultSettings 出厂默认值。
func DefaultSettings() Settings {
	return Settings{
		AI: AISettings{TimeoutSec: 60},
		Browser: BrowserSettings{
			StepDelayMS:     260,
			ActionTimeoutMS: 15000,
		},
	}
}
