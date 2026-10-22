// Package advise 负责「压测结果 → 分析结论 + 整改优化方案」。
//
// 配了大模型时由 AI 输出（喂给它的是结构化指标，不让它编数据）；
// 没配或失败时用内置规则引擎，按错误类型/延迟/接口分布给出可执行的整改清单。
package advise

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"kefu-loadtest/internal/aiclient"
	"kefu-loadtest/internal/engine"
)

// Result 是一次分析结果。
type Result struct {
	Source string `json:"source"` // ai / rules
	Model  string `json:"model"`
	Text   string `json:"text"` // markdown
	Error  string `json:"error,omitempty"`
}

// Run 生成分析与整改方案。
func Run(ctx context.Context, rep *engine.Report, ai *aiclient.Client) Result {
	if rep == nil {
		return Result{Source: "rules", Text: "还没有完成的压测报告，先跑一轮再分析。"}
	}
	if ai.Ready() {
		ctx2, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		text, err := aiAnalysis(ctx2, ai, rep)
		if err == nil && strings.TrimSpace(text) != "" {
			return Result{Source: "ai", Model: ai.Model(), Text: text}
		}
		rulesText := rulesAnalysis(rep)
		errMsg := ""
		if err != nil {
			errMsg = "大模型分析失败，已回退内置规则：" + err.Error()
		}
		return Result{Source: "rules", Text: rulesText, Error: errMsg}
	}
	return Result{Source: "rules", Text: rulesAnalysis(rep)}
}

const adviseSystemPrompt = `你是资深性能优化专家。用户刚跑完一轮 HTTP 压测，请基于给定的结构化指标写一份中文分析报告。

硬性要求：
1. 只能引用给定数据，不要编造没测过的指标、机器配置或日志内容；
2. 输出 Markdown，结构固定为四节：
   ## 一句话结论
   ## 瓶颈定位（引用具体接口与数字，说明判断依据）
   ## 整改优化方案（按 P0 必须改 / P1 建议改 / P2 可选 分组，每条写清「改什么、怎么改、改完怎么验证」）
   ## 复测建议（给阶梯并发的具体档位与观察指标）
3. 方案要落到可执行动作（索引、连接池、缓存、异步化、限流、扩容、参数调整等），避免空话；
4. 如果失败主要来自用例本身（路径/参数/鉴权不对），要明确指出「先修用例再谈性能」。`

func aiAnalysis(ctx context.Context, ai *aiclient.Client, rep *engine.Report) (string, error) {
	type scView struct {
		Name        string  `json:"name"`
		Method      string  `json:"method"`
		Path        string  `json:"path"`
		Concurrency int     `json:"concurrency"`
		Sent        int64   `json:"sent"`
		Ok          int64   `json:"ok"`
		Failed      int64   `json:"failed"`
		FailRate    float64 `json:"fail_rate"`
		AvgMs       float64 `json:"avg_ms"`
		P95Ms       float64 `json:"p95_ms"`
		P99Ms       float64 `json:"p99_ms"`
		MaxMs       float64 `json:"max_ms"`
	}
	view := struct {
		Target         string  `json:"target"`
		DurationSec    float64 `json:"duration_sec"`
		TotalConc      int     `json:"total_concurrency"`
		Sent           int64   `json:"sent"`
		Ok             int64   `json:"ok"`
		Failed         int64   `json:"failed"`
		Inflight       int64   `json:"inflight"`
		FailRate       float64 `json:"fail_rate"`
		AvgRPS         float64 `json:"avg_rps"`
		PeakRPS        float64 `json:"peak_rps"`
		AvgMs          float64 `json:"avg_ms"`
		P95Ms          float64 `json:"p95_ms"`
		P99Ms          float64 `json:"p99_ms"`
		MaxMs          float64 `json:"max_ms"`
		Errors         []struct {
			Kind   string `json:"kind"`
			Detail string `json:"detail"`
			Count  int64  `json:"count"`
		} `json:"errors"`
		Scenarios []scView `json:"scenarios"`
		Verdict   struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		} `json:"verdict"`
	}{
		Target:      rep.Config.Target,
		DurationSec: rep.DurationSec,
		TotalConc:   rep.Config.TotalConcurrency(),
		Sent:        rep.Summary.Sent,
		Ok:          rep.Summary.Ok,
		Failed:      rep.Summary.Failed,
		Inflight:    rep.Summary.Inflight,
		FailRate:    rep.Summary.FailRate,
		AvgRPS:      rep.Summary.AvgRPS,
		PeakRPS:     rep.Summary.PeakRPS,
		AvgMs:       rep.Summary.Latency.Avg,
		P95Ms:       rep.Summary.Latency.P95,
		P99Ms:       rep.Summary.Latency.P99,
		MaxMs:       rep.Summary.Latency.Max,
		Verdict: struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		}{Level: rep.Verdict.Level, Reasons: rep.Verdict.Reasons},
	}
	for _, e := range rep.Summary.Errors {
		view.Errors = append(view.Errors, struct {
			Kind   string `json:"kind"`
			Detail string `json:"detail"`
			Count  int64  `json:"count"`
		}{Kind: e.Kind, Detail: e.Detail, Count: e.Count})
	}
	for _, sc := range rep.Scenarios {
		view.Scenarios = append(view.Scenarios, scView{
			Name: sc.Name, Method: sc.Method, Path: sc.Path, Concurrency: sc.Concurrency,
			Sent: sc.Sent, Ok: sc.Ok, Failed: sc.Failed, FailRate: sc.FailRate,
			AvgMs: sc.Latency.Avg, P95Ms: sc.Latency.P95, P99Ms: sc.Latency.P99, MaxMs: sc.Latency.Max,
		})
	}
	payload, _ := json.MarshalIndent(view, "", "  ")
	user := "压测指标如下（JSON）：\n" + string(payload) + "\n\n请按要求输出分析报告。"
	return ai.Chat(ctx, adviseSystemPrompt, user)
}

/* ------------------------------ 规则引擎 ------------------------------ */

func rulesAnalysis(rep *engine.Report) string {
	s := rep.Summary
	var b strings.Builder

	// 结论
	b.WriteString("## 一句话结论\n")
	b.WriteString(conclusion(rep))
	b.WriteString("\n\n## 关键指标\n")
	b.WriteString("| 指标 | 数值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 目标地址 | %s |\n", rep.Config.Target)
	fmt.Fprintf(&b, "| 接口数量 / 总并发 | %d 个 / %d |\n", len(rep.Scenarios), rep.Config.TotalConcurrency())
	fmt.Fprintf(&b, "| 运行时长 | %.1f 秒 |\n", rep.DurationSec)
	fmt.Fprintf(&b, "| 发起 / 成功 / 失败 | %d / %d / %d（失败率 %.2f%%）|\n", s.Sent, s.Ok, s.Failed, s.FailRate*100)
	if s.Inflight > 0 {
		fmt.Fprintf(&b, "| 结束仍在途 | %d 条（不计入成功率）|\n", s.Inflight)
	}
	fmt.Fprintf(&b, "| 吞吐 | 平均 %.1f 条/秒，峰值 %.0f 条/秒 |\n", s.AvgRPS, s.PeakRPS)
	fmt.Fprintf(&b, "| 延迟 | 平均 %.0fms / P95 %.0fms / P99 %.0fms / 最大 %.0fms |\n",
		s.Latency.Avg, s.Latency.P95, s.Latency.P99, s.Latency.Max)
	fmt.Fprintf(&b, "| 稳定性判定 | %s |\n", rep.Verdict.Level)

	// 分接口
	if len(rep.Scenarios) > 1 {
		b.WriteString("\n## 分接口表现\n")
		b.WriteString("| 接口 | 并发 | 发起 | 成功 | 失败率 | 平均 | P95 | P99 |\n|---|---|---|---|---|---|---|---|\n")
		scs := make([]engine.ScenarioSummary, len(rep.Scenarios))
		copy(scs, rep.Scenarios)
		sort.Slice(scs, func(i, j int) bool { return scs[i].Latency.P95 > scs[j].Latency.P95 })
		for _, sc := range scs {
			fmt.Fprintf(&b, "| %s `%s %s` | %d | %d | %d | %.1f%% | %.0fms | %.0fms | %.0fms |\n",
				sc.Name, sc.Method, sc.Path, sc.Concurrency, sc.Sent, sc.Ok,
				sc.FailRate*100, sc.Latency.Avg, sc.Latency.P95, sc.Latency.P99)
		}
	}

	// 瓶颈定位
	b.WriteString("\n## 瓶颈定位\n")
	points := diagnose(rep)
	if len(points) == 0 {
		b.WriteString("- 本轮没有明显异常，延迟与失败率都在可接受区间，可以继续往上加并发找拐点。\n")
	} else {
		for _, p := range points {
			b.WriteString("- " + p + "\n")
		}
	}

	// 整改方案
	b.WriteString("\n## 整改优化方案\n")
	b.WriteString(plan(rep))

	// 复测
	b.WriteString("\n## 复测建议\n")
	fmt.Fprintf(&b, "1. 以当前总并发 %d 为基准，按 ½、1×、1.5×、2× 四档做阶梯（每档 60 秒，中间停 10 秒让系统排队消化）。\n",
		rep.Config.TotalConcurrency())
	b.WriteString("2. 每档记录：吞吐（条/秒）、P95、P99、失败率、错误类型；**吞吐不再上升而延迟陡增的那一档就是系统拐点**。\n")
	b.WriteString("3. 压测机与被测服务分开两台机器，避免压测机先成为瓶颈；每轮只改一个变量（改数据库/加缓存/调连接池）再对比。\n")
	return b.String()
}

func conclusion(rep *engine.Report) string {
	sum := rep.Summary
	switch {
	case sum.Ok+sum.Failed == 0:
		return "**无效结果**：一条请求都没成功，先把地址、路径、鉴权这三件事对齐（点「测这一个」逐条验证）。"
	case sum.FailRate >= 0.05:
		return fmt.Sprintf("**不通过**：失败率 %.1f%%，系统在当前并发下已经不可用，必须整改后再放量。", sum.FailRate*100)
	case sum.Latency.P99 >= 3000:
		return fmt.Sprintf("**不通过**：P99 %.0fms，超过 3 秒，用户体验会明显卡顿。", sum.Latency.P99)
	case sum.FailRate >= 0.01 || sum.Latency.P99 >= 1000:
		return fmt.Sprintf("**有风险**：失败率 %.2f%%、P99 %.0fms，当前并发已接近系统承受上限。",
			sum.FailRate*100, sum.Latency.P99)
	default:
		return fmt.Sprintf("**通过**：%d 条请求失败率 %.2f%%、P95 %.0fms，当前并发下表现正常，可以继续加压找拐点。",
			sum.Sent, sum.FailRate*100, sum.Latency.P95)
	}
}

func diagnose(rep *engine.Report) []string {
	var out []string
	s := rep.Summary

	// 最慢/最不稳的接口
	if len(rep.Scenarios) > 1 {
		slow := rep.Scenarios[0]
		bad := rep.Scenarios[0]
		for _, sc := range rep.Scenarios[1:] {
			if sc.Latency.P95 > slow.Latency.P95 {
				slow = sc
			}
			if sc.FailRate > bad.FailRate {
				bad = sc
			}
		}
		if slow.Latency.P95 > 0 && s.Latency.P95 > 0 && slow.Latency.P95 >= s.Latency.P95*1.5 {
			out = append(out, fmt.Sprintf("`%s %s`（%s）是主要瓶颈：P95 %.0fms，比整体 P95 高 %.0f%%，并发 %d",
				slow.Method, slow.Path, slow.Name, slow.Latency.P95,
				(slow.Latency.P95/s.Latency.P95-1)*100, slow.Concurrency))
		}
		if bad.FailRate > 0 && bad.FailRate >= s.FailRate*1.5 {
			out = append(out, fmt.Sprintf("`%s %s`（%s）最不稳定：失败率 %.1f%%，失败 %d 次",
				bad.Method, bad.Path, bad.Name, bad.FailRate*100, bad.Failed))
		}
	}

	// 错误类型 → 判断
	for _, e := range s.Errors {
		switch {
		case e.Kind == "timeout":
			out = append(out, fmt.Sprintf("存在 %d 次请求超时：服务端在超时时间内没处理完，通常是慢 SQL、锁等待或连接池打满", e.Count))
		case e.Kind == "conn_refused" || e.Kind == "conn_reset" || e.Kind == "conn_closed":
			out = append(out, fmt.Sprintf("存在 %d 次连接被拒/被重置：进程可能崩溃重启、监听 backlog 打满或端口/句柄耗尽", e.Count))
		case strings.HasPrefix(e.Kind, "http_5"):
			out = append(out, fmt.Sprintf("存在 %d 次服务端 5xx（%s）：先看服务端错误日志定位 panic / 空指针 / DB 异常", e.Count, e.Kind))
		case e.Kind == "http_429":
			out = append(out, fmt.Sprintf("存在 %d 次限流（429）：网关/应用限流阈值低于目标并发，压测结论需在放开限流后重测", e.Count))
		case e.Kind == "http_404" || e.Kind == "biz_error":
			out = append(out, fmt.Sprintf("存在 %d 次用例类失败（%s）：属路径/参数/鉴权问题，应先修用例再看性能", e.Count, e.Kind))
		case strings.HasPrefix(e.Kind, "ws_"):
			out = append(out, fmt.Sprintf("WebSocket 异常 %d 次（%s）：关注心跳、连接数上限与推送广播开销", e.Count, e.Kind))
		}
	}

	// 延迟形态
	if s.Latency.P95 > 0 && s.Latency.Max > s.Latency.P95*3 {
		out = append(out, fmt.Sprintf("尾部延迟突出（P95 %.0fms，最大 %.0fms）：存在偶发长尾，通常是锁竞争、GC、慢查询或冷缓存",
			s.Latency.P95, s.Latency.Max))
	}
	if s.Latency.P99 >= 1000 && len(out) == 0 {
		out = append(out, fmt.Sprintf("延迟偏高（P95 %.0fms / P99 %.0fms）：没有明显报错，说明服务端还没崩，但处理已经开始排队",
			s.Latency.P95, s.Latency.P99))
	}
	if s.Inflight > 0 {
		out = append(out, fmt.Sprintf("结束时有 %d 条请求未返回：说明服务端已排队，实际容量低于本轮并发", s.Inflight))
	}
	return out
}

func plan(rep *engine.Report) string {
	s := rep.Summary
	var p0, p1, p2 []string

	hasTimeout := false
	hasConnErr := false
	has5xx := false
	has429 := false
	hasCaseErr := false
	for _, e := range s.Errors {
		switch {
		case e.Kind == "timeout":
			hasTimeout = true
		case e.Kind == "conn_refused" || e.Kind == "conn_reset" || e.Kind == "conn_closed":
			hasConnErr = true
		case strings.HasPrefix(e.Kind, "http_5"):
			has5xx = true
		case e.Kind == "http_429":
			has429 = true
		case e.Kind == "http_404" || e.Kind == "biz_error":
			hasCaseErr = true
		}
	}

	if hasCaseErr {
		p0 = append(p0, "**先修用例**：用页面上的「测这一个」逐条验证接口路径、参数与鉴权，把 4xx/业务码错误消掉再谈性能")
	}
	slow := s.Latency.P99 >= 1000 || s.Latency.P95 >= 500 || s.Inflight > 0
	if hasTimeout || slow {
		p1 = append(p1,
			"**慢查询治理**：打开数据库慢查询日志（阈值 100ms），按 `conversation_id`、`customer_id`、`created_at` 逐一确认索引命中",
			"**热点缓存**：会话、客户资料、自动回复规则、租户配置放 Redis，减少每次请求的重复查询",
			"**异步化**：消息落库后把「推送客服端/统计累加/自动回复匹配」拆成异步任务，缩短主链路耗时")
	}
	if s.Latency.P99 >= 3000 {
		p0 = append(p0, fmt.Sprintf(
			"**先定位长尾**：P99 %.0fms 说明有慢请求在拖后腿，按接口拉 slow log（>500ms）定位到具体 SQL 或 RPC 再优化，别急着扩容",
			s.Latency.P99))
	}
	if hasTimeout {
		p0 = append(p0,
			"**数据库选型**：SQLite 单写锁是常见天花板，压测并发一上来就排队；线上应换 MySQL/PostgreSQL，并开 WAL 或读写分离",
			"**连接池**：把 DB 连接池 max_open 调到「并发数 × 1.5」左右，并设置合理 max_idle，避免每次请求新建连接")
	}
	if hasConnErr {
		p0 = append(p0,
			"**进程稳定性**：查服务端日志是否有 panic/OOM；确认 `ulimit -n`、监听 backlog、TIME_WAIT 数量",
			"**压测机侧**：确认不是压测机端口耗尽（保持 HTTP keep-alive，或用多台压测机）")
	}
	if has5xx {
		p0 = append(p0, "**错误日志定位**：按 5xx 时间点拉服务端日志，定位到具体堆栈后针对性修（空指针、事务失败、连接池耗尽）")
	}
	if has429 {
		p1 = append(p1, "**限流策略**：明确限流阈值是否匹配业务预期；若要压出真实容量，先在测试环境放开限流，再单独验证限流行为")
	}
	if s.Latency.P95 >= 500 || s.FailRate >= 0.01 || s.Inflight > 0 {
		p1 = append(p1,
			"**引入队列削峰**：高峰期把客户消息先入队（Kafka/Redis List）再异步落库，避免写入被同步阻塞",
			"**水平扩容**：多实例 + 负载均衡（WebSocket 需会话粘滞或广播走 Redis Pub/Sub），按单实例拐点的 70% 规划容量")
	}
	p2 = append(p2,
		"**可观测性**：接口维度采集 P95/P99、错误率、慢请求采样，配置阈值告警，便于提前发现容量拐点",
		"**压测常态化**：每次发版跑一轮阶梯压测（本工具可直接接 CI，退出码非 0 即视为性能回归）",
		"**容量基线**：把本轮「并发-吞吐-延迟」数据存档，作为后续优化的对比基线")

	if len(p0) == 0 && len(p1) == 0 {
		p0 = append(p0, "当前指标健康：无需整改，直接进入阶梯加压找拐点，并为容量规划留出 30% 余量")
	}

	var b strings.Builder
	writeGroup := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n### " + title + "\n")
		for i, it := range items {
			fmt.Fprintf(&b, "%d. %s\n", i+1, it)
		}
	}
	writeGroup("P0 必须改（不改没法放量）", p0)
	writeGroup("P1 建议改（决定容量上限）", p1)
	writeGroup("P2 可选（长期收益）", p2)
	return b.String()
}
