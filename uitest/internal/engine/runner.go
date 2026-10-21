// Package engine 是执行引擎：把一条录制「跑」成一次真实浏览器操作。
//
// 一次执行 = 打开内置浏览器标签页 → 注入共享运行时 → 逐步执行（变量替换 + 多级定位
// + 自愈）→ 逐步落库并通过事件总线实时推送日志 → 收尾（截图/结论）。
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	jsruntime "uitest-server/runtime"

	"uitest-server/internal/browser"
	"uitest-server/internal/bus"
	"uitest-server/internal/model"
	"uitest-server/internal/store"
)

// Runner 执行引擎。
type Runner struct {
	store *store.DB
	br    *browser.Manager
	bus   *bus.Bus

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// New 创建执行引擎。
func New(db *store.DB, br *browser.Manager, b *bus.Bus) *Runner {
	return &Runner{store: db, br: br, bus: b, cancels: map[string]context.CancelFunc{}}
}

/* ------------------------------------------------------------ 生命周期 */

// Start 异步执行一次运行（runID 必须已存在于存储）。
func (r *Runner) Start(runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancels[runID] = cancel
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.cancels, runID)
			r.mu.Unlock()
			cancel()
		}()
		r.execute(ctx, runID)
	}()
}

// Cancel 取消一次运行。
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	cancel, ok := r.cancels[runID]
	r.mu.Unlock()
	if !ok {
		return errors.New("该运行不在执行中")
	}
	cancel()
	return nil
}

/* ---------------------------------------------------------------- 执行 */

// actionResult 对应 runtime/uitest-keys.js 里 runAction 的返回结构。
type actionResult struct {
	OK       bool     `json:"ok"`
	Message  string   `json:"message"`
	Used     string   `json:"used"`
	Healed   bool     `json:"healed"`
	Tried    []string `json:"tried"`
	CSS      string   `json:"css"`
	Tag      string   `json:"tag"`
	Label    string   `json:"label"`
	URL      string   `json:"url"`
	Navigate string   `json:"navigate"`
	Snippet  string   `json:"snippet"`
}

func (r *Runner) execute(ctx context.Context, runID string) {
	run, err := r.store.GetRun(runID)
	if err != nil {
		return
	}
	rec, err := r.store.GetRecording(run.RecordingID)
	if err != nil {
		r.finish(runID, "failed", "录制不存在: "+err.Error())
		return
	}

	_, _ = r.store.MutateRun(runID, func(run *model.Run) { run.TotalSteps = len(rec.Steps) })
	r.setStatus(runID, "running")
	r.logf(runID, "info", 0, "开始执行「%s」，共 %d 步，数据来源：%s", rec.Name, len(rec.Steps), run.DatasetSource)
	if len(run.Dataset) > 0 {
		r.logf(runID, "info", 0, "本次数据准备：%s", datasetSummary(run.Dataset))
	}

	openCtx, cancelOpen := context.WithTimeout(ctx, 150*time.Second)
	page, err := r.br.OpenPage(openCtx, "about:blank")
	cancelOpen()
	if err != nil {
		r.finish(runID, "failed", "启动内置浏览器失败："+err.Error())
		return
	}
	defer page.Close(context.Background())
	r.logf(runID, "info", 0, "内置浏览器已就绪，标签页 targetId=%s", page.TargetID)

	startURL := strings.TrimSpace(rec.StartURL)
	if startURL == "" {
		for _, s := range rec.Steps {
			if s.Action == "navigate" && strings.TrimSpace(s.Value) != "" {
				startURL = s.Value
				break
			}
		}
	}
	if startURL != "" {
		navCtx, cancelNav := context.WithTimeout(ctx, 60*time.Second)
		navErr := page.Navigate(navCtx, startURL)
		cancelNav()
		if navErr != nil {
			r.finish(runID, "failed", "打开起始页失败："+navErr.Error())
			return
		}
		r.logf(runID, "info", 0, "已打开起始页 %s", startURL)
	}
	if err := r.ensureRuntime(ctx, page); err != nil {
		r.finish(runID, "failed", "注入页面运行时失败："+err.Error())
		return
	}

	failed := 0
	for _, step := range rec.Steps {
		if ctx.Err() != nil {
			r.logf(runID, "warn", step.Index, "已取消，停止后续步骤")
			r.finish(runID, "canceled", "")
			return
		}
		res := r.execStep(ctx, runID, page, step, run.Dataset)
		if res.Status == "failed" {
			failed++
			if step.Optional {
				r.logf(runID, "warn", step.Index, "该步骤标记为可跳过，继续执行")
				continue
			}
			r.finish(runID, "failed", fmt.Sprintf("第 %d 步失败：%s", step.Index, res.Message))
			return
		}
		// 步骤间隔，给前端渲染/请求留出时间
		delay := time.Duration(r.stepDelayMS()) * time.Millisecond
		wait := delay
		if step.WaitMS > 0 {
			wait += time.Duration(step.WaitMS) * time.Millisecond
		}
		if wait > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
		}
	}

	if failed > 0 {
		r.finish(runID, "passed", fmt.Sprintf("全部步骤执行完毕（%d 个失败步骤被标记为可跳过）", failed))
		return
	}
	r.finish(runID, "passed", "")
}

func (r *Runner) execStep(ctx context.Context, runID string, page *browser.Page, step model.Step, ds map[string]string) model.StepResult {
	started := time.Now()
	value := Substitute(step.Value, ds)
	annotation := step.Annotation
	if annotation == "" {
		annotation = step.Action
	}
	res := model.StepResult{
		Index:      step.Index,
		StepID:     step.ID,
		Action:     step.Action,
		Annotation: annotation,
		ValueUsed:  value,
		Status:     "passed",
	}
	r.logf(runID, "info", step.Index, "▶ %s", annotation)

	timeout := time.Duration(r.actionTimeoutMS()) * time.Millisecond
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var failure string
	switch step.Action {
	case "navigate":
		if strings.TrimSpace(value) == "" {
			failure = "导航步骤缺少 URL"
			break
		}
		if err := page.Navigate(stepCtx, value); err != nil {
			failure = "导航失败：" + err.Error()
			break
		}
		res.LocatorUsed = "url=" + value
		if err := r.ensureRuntime(stepCtx, page); err != nil {
			failure = "注入运行时失败：" + err.Error()
		}
	case "upload":
		failure = r.execUpload(stepCtx, page, step, value, &res)
	default:
		if err := r.ensureRuntime(stepCtx, page); err != nil {
			failure = "注入运行时失败：" + err.Error()
			break
		}
		req := map[string]any{
			"action":    step.Action,
			"locator":   step.Locator,
			"target":    step.Target,
			"value":     value,
			"timeoutMs": r.actionTimeoutMS(),
		}
		rawReq, _ := json.Marshal(req)
		var out actionResult
		expr := fmt.Sprintf(`(async () => {
  if (!window.__uitestKeys) return {ok:false, message:"页面运行时未注入（可能是 CSP 拦截或页面已跳走）"};
  return await window.__uitestKeys.runAction(%s);
})()`, rawReq)
		if err := page.EvalInto(stepCtx, expr, &out); err != nil {
			failure = "执行异常：" + err.Error()
			break
		}
		res.LocatorUsed = out.Used
		res.Healed = out.Healed
		if !out.OK {
			failure = out.Message
			if len(out.Tried) > 0 {
				res.HealFrom = strings.Join(out.Tried, " → ")
			}
			break
		}
		if out.Message != "" {
			res.Message = out.Message
		}
		if out.Healed {
			res.HealFrom = strings.Join(out.Tried, " → ")
			r.logf(runID, "warn", step.Index, "定位失效，已自愈：%s（原定位 %s）", out.Used, res.HealFrom)
		}
	}

	// 断言（步骤自带 expect，或独立的 assert 动作）
	if failure == "" && step.Action != "assert" && strings.TrimSpace(step.Expect) != "" {
		expect := Substitute(step.Expect, ds)
		var exp actionResult
		rawExp, _ := json.Marshal(expect)
		expr := fmt.Sprintf(`(async () => { const r = window.__uitestKeys.checkExpect(%s); return {ok:r.ok, message:r.message||"", snippet:r.snippet||""}; })()`, rawExp)
		if err := page.EvalInto(stepCtx, expr, &exp); err != nil {
			failure = "断言执行异常：" + err.Error()
		} else if !exp.OK {
			failure = "断言未通过：" + exp.Message
			res.Expected = expect
			res.Actual = exp.Snippet
		} else {
			res.Message = strings.TrimSpace(res.Message + " · 断言通过")
		}
	}

	res.DurationMS = time.Since(started).Milliseconds()
	if failure != "" {
		res.Status = "failed"
		res.Message = failure
		if shot := r.screenshot(stepCtx, runID, page, step.Index); shot != "" {
			res.Screenshot = shot
		}
		r.logf(runID, "error", step.Index, "✗ %s", failure)
	} else {
		r.logf(runID, "info", step.Index, "✓ %s（%dms）", firstNonEmpty(res.Message, annotation), res.DurationMS)
	}
	r.appendStepResult(runID, res)
	return res
}

// execUpload 处理文件上传：先定位元素拿唯一选择器，再用 CDP 注入本地文件。
func (r *Runner) execUpload(ctx context.Context, page *browser.Page, step model.Step, value string, res *model.StepResult) string {
	req := map[string]any{"action": "upload", "locator": step.Locator, "target": step.Target, "value": value}
	rawReq, _ := json.Marshal(req)
	expr := fmt.Sprintf(`(async () => {
  if (!window.__uitestKeys) return {ok:false, message:"页面运行时未注入"};
  return await window.__uitestKeys.runAction(%s);
})()`, rawReq)
	var out actionResult
	if err := page.EvalInto(ctx, expr, &out); err != nil {
		return "执行异常：" + err.Error()
	}
	res.LocatorUsed = out.Used
	res.Healed = out.Healed
	if !out.OK {
		return out.Message
	}
	css := out.CSS
	if out.Tag != "" && out.Tag != "input" {
		css = css + " input[type=file]"
	}
	file := strings.TrimSpace(value)
	if file == "" || !fileExists(file) {
		gen, err := r.makeSampleImage()
		if err != nil {
			return "无法准备上传文件：" + err.Error()
		}
		file = gen
		res.Message = "已自动生成示例图片用于上传"
	}
	if err := page.SetFileInput(ctx, css, []string{file}); err != nil {
		return "设置上传文件失败：" + err.Error()
	}
	res.ValueUsed = file
	return ""
}

func (r *Runner) makeSampleImage() (string, error) {
	dir := filepath.Join(r.store.Dir(), "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "auto-test-upload.png")
	if fileExists(path) {
		return path, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, 240, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 240; x++ {
			img.Set(x, y, color.RGBA{R: uint8(79), G: uint8(70), B: uint8(229), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Runner) screenshot(ctx context.Context, runID string, page *browser.Page, index int) string {
	if page == nil {
		return ""
	}
	shotCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	data, err := page.Screenshot(shotCtx)
	if err != nil || len(data) == 0 {
		return ""
	}
	rel := filepath.Join("data", "runs", runID)
	abs := filepath.Join(r.store.Dir(), "runs", runID)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return ""
	}
	name := fmt.Sprintf("step_%02d.png", index)
	if err := os.WriteFile(filepath.Join(abs, name), data, 0o644); err != nil {
		return ""
	}
	_ = ctx
	return filepath.ToSlash(filepath.Join(rel, name))
}

// ensureRuntime 保证页面里有共享运行时（幂等，版本不符则重装）。
func (r *Runner) ensureRuntime(ctx context.Context, page *browser.Page) error {
	var version string
	if err := page.EvalInto(ctx, `(window.__uitestKeys && window.__uitestKeys.version) || ""`, &version); err != nil {
		return err
	}
	if version != "" {
		return nil
	}
	if _, err := page.Eval(ctx, jsruntime.KeysJS); err != nil {
		return err
	}
	var markers any
	_ = page.EvalInto(ctx, `window.__uitestKeys.markAll()`, &markers)
	return nil
}

/* --------------------------------------------------------- 存储/日志辅助 */

func (r *Runner) setStatus(runID, status string) {
	now := time.Now()
	_, _ = r.store.MutateRun(runID, func(run *model.Run) {
		run.Status = status
		if status == "running" && run.StartedAt.IsZero() {
			run.StartedAt = now
		}
		if status != "running" && status != "queued" {
			run.FinishedAt = now
		}
	})
	r.bus.Publish(runID, bus.Event{Type: "status", Status: status})
}

func (r *Runner) finish(runID, status, errMsg string) {
	_, _ = r.store.MutateRun(runID, func(run *model.Run) {
		run.Status = status
		run.FinishedAt = time.Now()
		if errMsg != "" {
			run.Error = errMsg
		}
		healables := 0
		passed := 0
		for _, sr := range run.StepResults {
			if sr.Healed {
				healables++
			}
			if sr.Status == "passed" {
				passed++
			}
		}
		run.Healables = healables
		run.PassedSteps = passed
		if run.TotalSteps == 0 {
			run.TotalSteps = len(run.StepResults)
		}
	})
	if errMsg != "" {
		r.logf(runID, "error", 0, "%s", errMsg)
	}
	level := "info"
	switch status {
	case "failed":
		level = "error"
	case "canceled":
		level = "warn"
	}
	r.logf(runID, level, 0, "执行结束：%s", statusLabel(status))
	r.bus.Publish(runID, bus.Event{Type: "status", Status: status})
	r.bus.Publish(runID, bus.Event{Type: "end", Status: status})
}

func (r *Runner) logf(runID, level string, index int, format string, args ...any) {
	entry := model.LogEntry{
		TS:      time.Now(),
		Level:   level,
		Index:   index,
		Message: fmt.Sprintf(format, args...),
	}
	_, _ = r.store.MutateRun(runID, func(run *model.Run) {
		run.Logs = append(run.Logs, entry)
		if len(run.Logs) > 4000 {
			run.Logs = run.Logs[len(run.Logs)-4000:]
		}
	})
	r.bus.Publish(runID, bus.Event{Type: "log", Log: &entry})
}

func (r *Runner) appendStepResult(runID string, sr model.StepResult) {
	_, _ = r.store.MutateRun(runID, func(run *model.Run) {
		run.StepResults = append(run.StepResults, sr)
	})
	r.bus.Publish(runID, bus.Event{Type: "step", Step: &sr})
}

func statusLabel(s string) string {
	switch s {
	case "passed":
		return "通过"
	case "failed":
		return "失败"
	case "canceled":
		return "已取消"
	case "running":
		return "执行中"
	default:
		return s
	}
}

func datasetSummary(ds map[string]string) string {
	keys := make([]string, 0, len(ds))
	for k := range ds {
		keys = append(keys, k)
	}
	// 稳定输出顺序，便于日志比对
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+ds[k])
	}
	return strings.Join(parts, ", ")
}

func (r *Runner) stepDelayMS() int {
	v := r.store.Settings().Browser.StepDelayMS
	if v <= 0 {
		v = 260
	}
	return v
}

func (r *Runner) actionTimeoutMS() int {
	v := r.store.Settings().Browser.ActionTimeoutMS
	if v <= 0 {
		v = 15000
	}
	return v
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func firstNonEmpty(list ...string) string {
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

/* ------------------------------------------------------------ 变量替换 */

var varRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_\-]+)(?::(\d+))?\s*\}\}`)

// Substitute 把 {{key}} 替换成数据准备里的值，并支持内置变量：
// {{now}} {{today}} {{datetime}} {{timestamp}} {{uuid}} {{random:8}}
func Substitute(tpl string, ds map[string]string) string {
	if !strings.Contains(tpl, "{{") {
		return tpl
	}
	now := time.Now()
	return varRe.ReplaceAllStringFunc(tpl, func(m string) string {
		groups := varRe.FindStringSubmatch(m)
		key := groups[1]
		arg := groups[2]
		if v, ok := ds[key]; ok {
			return v
		}
		switch key {
		case "now":
			return now.Format("2006-01-02 15:04:05")
		case "today":
			return now.Format("2006-01-02")
		case "datetime":
			return now.Format("20060102150405")
		case "date":
			return now.Format("2006-01-02")
		case "timestamp":
			return fmt.Sprintf("%d", now.Unix())
		case "uuid":
			return randHex(8)
		case "random":
			n := 6
			if arg != "" {
				if v, err := parseInt(arg); err == nil && v > 0 && v <= 64 {
					n = v
				}
			}
			return randHex(n)
		default:
			return m // 未知变量保留原样，便于在运行日志里发现遗漏
		}
	})
}

func parseInt(s string) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("非法数字")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())[:n]
	}
	s := hex.EncodeToString(buf)
	if len(s) > n {
		s = s[:n]
	}
	return s
}
