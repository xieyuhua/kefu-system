package engine

import (
	"fmt"
	"runtime"
	"sort"
	"time"

	"kefu-loadtest/internal/config"
	"kefu-loadtest/internal/metrics"
)

// EnvInfo 记录压测机环境，便于对比不同机器上的结论。
type EnvInfo struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	NumCPU    int    `json:"num_cpu"`
}

// Verdict 是稳定性结论。
type Verdict struct {
	Pass    bool     `json:"pass"`
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

// Report 是一次压测的最终报告。
type Report struct {
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at"`
	DurationSec   float64           `json:"duration_sec"`
	StoppedByUser bool              `json:"stopped_by_user"`
	Config        config.Config     `json:"config"`
	Summary       metrics.Summary   `json:"summary"`
	Scenarios     []ScenarioSummary `json:"scenarios"`
	Verdict       Verdict           `json:"verdict"`
	Env           EnvInfo           `json:"env"`
	Files         []string          `json:"files,omitempty"`
	AdviceSource  string            `json:"advice_source,omitempty"`
}

// BuildReport 汇总最终报告并给出稳定性判定。
func BuildReport(r *Run, stoppedByUser bool) *Report {
	sum := r.Summary()
	rep := &Report{
		StartedAt:     r.startedAt,
		FinishedAt:    time.Now(),
		DurationSec:   sum.ElapsedSec,
		StoppedByUser: stoppedByUser,
		Config:        r.cfg,
		Summary:       sum.Summary,
		Scenarios:     sum.Scenarios,
		Env: EnvInfo{
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS + "/" + runtime.GOARCH,
			NumCPU:    runtime.NumCPU(),
		},
	}
	rep.Verdict = judge(sum.Summary, sum.Scenarios)
	return rep
}

func judge(sum metrics.Summary, scenarios []ScenarioSummary) Verdict {
	v := Verdict{Pass: true, Level: "通过"}

	// 硬性指标：失败率（分母是已完成的请求）
	if sum.Ok+sum.Failed > 0 {
		switch {
		case sum.FailRate >= 0.05:
			v.Pass = false
			v.Level = "不通过"
			v.Reasons = append(v.Reasons,
				fmt.Sprintf("整体失败率 %.2f%%（%d/%d），超过 5%% 阈值，系统在当前并发下已不可用",
					sum.FailRate*100, sum.Failed, sum.Ok+sum.Failed))
		case sum.FailRate >= 0.01:
			v.Level = "有风险"
			v.Reasons = append(v.Reasons,
				fmt.Sprintf("整体失败率 %.2f%%（%d/%d），超过 1%% 关注线",
					sum.FailRate*100, sum.Failed, sum.Ok+sum.Failed))
		}
	} else {
		v.Pass = false
		v.Level = "无效"
		v.Reasons = append(v.Reasons, "一条请求都没成功发出去，请检查目标地址、接口路径与鉴权")
	}

	if sum.Inflight > 0 {
		if v.Pass {
			v.Level = "有风险"
		}
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("压测结束时有 %d 条请求还没返回（被停止或到时长仍在途），不计入成功率", sum.Inflight))
	}

	// 延迟
	switch {
	case sum.Latency.P99 >= 3000:
		v.Pass = false
		v.Level = "不通过"
		v.Reasons = append(v.Reasons, fmt.Sprintf("整体 P99 延迟 %.0fms，超过 3000ms，用户可感知卡顿", sum.Latency.P99))
	case sum.Latency.P99 >= 1000:
		if v.Pass {
			v.Level = "有风险"
		}
		v.Reasons = append(v.Reasons, fmt.Sprintf("整体 P99 延迟 %.0fms，超过 1000ms 关注线", sum.Latency.P99))
	}
	if sum.Latency.Max >= 10000 {
		if v.Pass {
			v.Level = "有风险"
		}
		v.Reasons = append(v.Reasons, fmt.Sprintf("最长请求耗时 %.0fms，存在接近超时的慢请求", sum.Latency.Max))
	}

	// 分接口：找出最差的
	if len(scenarios) > 1 {
		slow := scenarios[0]
		bad := scenarios[0]
		for _, sc := range scenarios[1:] {
			if sc.Latency.P95 > slow.Latency.P95 {
				slow = sc
			}
			if sc.FailRate > bad.FailRate {
				bad = sc
			}
		}
		if slow.Latency.P95 >= 500 {
			v.Reasons = append(v.Reasons, fmt.Sprintf("最慢的是「%s」（%s %s），P95 %.0fms，优化应优先看它",
				slow.Name, slow.Method, slow.Path, slow.Latency.P95))
		}
		if bad.FailRate >= 0.01 {
			if v.Pass {
				v.Level = "有风险"
			}
			v.Reasons = append(v.Reasons, fmt.Sprintf("失败率最高的是「%s」（%s %s），%.1f%%，共失败 %d 次",
				bad.Name, bad.Method, bad.Path, bad.FailRate*100, bad.Failed))
		}
	}

	// 初始化与长连接
	if sum.InitFailed > 0 {
		if v.Pass {
			v.Level = "有风险"
		}
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("客户初始化失败 %d 次（成功 %d 次），会话建立可能是瓶颈", sum.InitFailed, sum.InitOk))
	}
	if sum.WsConnects > 0 && sum.WsDisconnects > 0 {
		rate := float64(sum.WsDisconnects) / float64(sum.WsConnects)
		if rate >= 0.2 {
			if v.Pass {
				v.Level = "有风险"
			}
			v.Reasons = append(v.Reasons,
				fmt.Sprintf("WebSocket 断线 %d 次 / 连接 %d 次（%.0f%%），实时推送不稳定",
					sum.WsDisconnects, sum.WsConnects, rate*100))
		}
	}

	if len(v.Reasons) == 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf(
			"共发起 %d 条请求，全部成功，P95 %.0fms、P99 %.0fms，实时通道未见异常",
			sum.Sent, sum.Latency.P95, sum.Latency.P99))
	}
	return v
}

// sortScenariosByP95 供报告与建议使用：按 P95 从慢到快排序。
func sortScenariosByP95(scenarios []ScenarioSummary) []ScenarioSummary {
	out := make([]ScenarioSummary, len(scenarios))
	copy(out, scenarios)
	sort.Slice(out, func(i, j int) bool { return out[i].Latency.P95 > out[j].Latency.P95 })
	return out
}
