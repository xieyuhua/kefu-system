// Package discover 负责「给一个地址，自动勘察出可压测的接口清单」。
//
// 勘察手段（都只做只读探测，不会写数据）：
//  1. 目标探活：拿到状态码、响应时间、内容类型；
//  2. OpenAPI/Swagger 探测：有就按契约把 path + method 全部列出来；
//  3. 已知模式识别：命中客服系统访客链路（/api/guest/init 返回 code=0）时，
//     直接给出访客发消息 / 初始化 / 拉消息 / 客服登录 / 统计等候选；
//  4. 页面与脚本扫描：从 HTML/JS 里捞 /api/... 路径；
//  5. 配了大模型时，把上面的「事实 + 候选」交给 AI 整理成中文接口清单，
//     补上用途说明、风险等级、建议并发与间隔；没配或失败就用内置规则的结果。
package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"kefu-loadtest/internal/aiclient"
	"kefu-loadtest/internal/config"
)

// Fact 是一条勘察事实，展示给用户看，也作为 AI 的输入依据。
type Fact struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	Level  string `json:"level"` // ok / info / warn / error
}

// Candidate 是一个候选接口（带是否推荐勾选的理由）。
type Candidate struct {
	config.Scenario
	Recommended bool   `json:"recommended"`
	Reason      string `json:"reason"`
}

// PageInfo 记录「你填的地址」被怎么解析成接口基址与渠道参数。
type PageInfo struct {
	InputURL string `json:"input_url"`
	BaseURL  string `json:"base_url"`
	PageURL  string `json:"page_url,omitempty"`
	Channel  string `json:"channel,omitempty"`
	UID      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Tenant   string `json:"tenant,omitempty"`
	Note     string `json:"note"`
}

// Result 是勘察结果。
type Result struct {
	Target     string      `json:"target"`
	Channel    string      `json:"channel"`
	Page       PageInfo    `json:"page"`
	Facts      []Fact      `json:"facts"`
	Candidates []Candidate `json:"candidates"`
	Hints      []string    `json:"hints"`
	Source     string      `json:"source"` // ai / rules
	Model      string      `json:"model"`
	Error      string      `json:"error,omitempty"`
	ElapsedMs  float64     `json:"elapsed_ms"`
}

const maxBody = 512 << 10

// splitTarget 把你填的地址拆成「接口基址 + 要扫描的页面地址 + 渠道参数」。
//
// 支持这几种写法都能正确解析：
//
//	http://host:8480
//	http://host:8480/kefu/?channel=kefu_1edcf1a9&uid=20261022094940#/
//	https://host/admin/#/channels
//	https://api.example.com/api/v1        （带路径前缀的接口基址，会保留）
func splitTarget(raw string) (base string, pages []string, info PageInfo) {
	raw = strings.TrimSpace(raw)
	info.InputURL = raw
	base, pageURL := config.SplitTarget(raw)
	info.BaseURL = base

	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		q := u.Query()
		info.Channel = strings.TrimSpace(q.Get("channel"))
		info.UID = strings.TrimSpace(q.Get("uid"))
		info.Nickname = strings.TrimSpace(q.Get("nickname"))
		info.Tenant = strings.TrimSpace(q.Get("tenant"))
	}

	switch {
	case base == "":
		pages = []string{raw}
		info.Note = "地址为空或无法解析"
	case pageURL == "" || pageURL == base+"/":
		pages = []string{base + "/"}
		info.Note = fmt.Sprintf("识别为接口基址：%s", base)
	default:
		info.PageURL = pageURL
		pages = []string{pageURL, base + "/"}
		info.Note = fmt.Sprintf("识别为前端页面地址：接口基址用 %s，页面 %s", base, pageURL)
	}

	extras := make([]string, 0, 4)
	if info.Channel != "" {
		extras = append(extras, "渠道="+info.Channel)
	}
	if info.UID != "" {
		extras = append(extras, "客户ID="+info.UID)
	}
	if info.Tenant != "" {
		extras = append(extras, "租户="+info.Tenant)
	}
	if len(extras) > 0 {
		info.Note += "；从地址参数取到 " + strings.Join(extras, "、")
	}
	return base, pages, info
}

// Run 执行一次接口勘察。
func Run(ctx context.Context, target, channel string, ai *aiclient.Client, concurrency int) Result {
	start := time.Now()
	base, pages, page := splitTarget(target)
	if page.Channel != "" {
		channel = page.Channel // 地址里带的渠道优先级最高
	}
	if channel == "" {
		channel = "web"
	}
	page.Channel = channel
	if concurrency <= 0 {
		concurrency = 10
	}
	res := Result{Target: base, Channel: channel, Page: page, Source: "rules"}

	client := &http.Client{Timeout: 12 * time.Second}

	// 0. 地址解析结论（先说清「你填的地址被怎么理解」）
	res.Facts = append(res.Facts, Fact{Kind: "地址解析", Detail: page.Note, Level: "info"})

	// 1. 探活
	if body, status, ms, ct, err := fetch(ctx, client, base+"/"); err != nil {
		res.Facts = append(res.Facts, Fact{Kind: "探活", Detail: "访问失败：" + err.Error(), Level: "error"})
	} else {
		res.Facts = append(res.Facts, Fact{
			Kind:   "探活",
			Detail: fmt.Sprintf("GET %s/ → HTTP %d，%.0fms，Content-Type %s，%d 字节", base, status, ms, ct, len(body)),
			Level:  "ok",
		})
	}

	// 2. OpenAPI / Swagger
	apiScenarios, apiFacts := scanOpenAPI(ctx, client, base)
	res.Facts = append(res.Facts, apiFacts...)

	// 3. 已知模式（客服系统访客链路）
	guestScenarios, guestFacts := scanGuest(ctx, client, base, channel, page.UID, concurrency)
	res.Facts = append(res.Facts, guestFacts...)

	// 4. 页面/脚本扫描（页面地址 + 首页都扫）
	scanScenarios, scanFacts := scanFrontend(ctx, client, base, pages)
	res.Facts = append(res.Facts, scanFacts...)

	// 汇总候选（按路径去重，后来的补充说明不覆盖已有的）
	cands := mergeCandidates(apiScenarios, guestScenarios, scanScenarios)
	if len(cands) == 0 {
		cands = append(cands, Candidate{
			Scenario: config.Scenario{
				ID:          "sc_root",
				Name:        "首页探活",
				Method:      "GET",
				Path:        "/",
				Concurrency: concurrency,
				IntervalMs:  1000,
				JitterMs:    100,
				Risk:        config.RiskRead,
				Note:        "没有识别到业务接口，只能压首页；建议提供 OpenAPI 文档或接口路径",
			},
			Recommended: true,
			Reason:      "兜底场景：只验证静态页/网关吞吐",
		})
	}

	// 5. AI 整理
	if ai.Ready() {
		refined, hints, err := refineWithAI(ctx, ai, res, cands)
		if err != nil {
			res.Error = "大模型整理失败，已回退内置规则：" + err.Error()
			res.Hints = append(res.Hints, "提示：可在右上角「AI 设置」里检查 base_url / model / key")
		} else {
			cands = refined
			res.Hints = append(res.Hints, hints...)
			res.Source = "ai"
			res.Model = ai.Model()
		}
	} else {
		res.Hints = append(res.Hints, "未配置大模型，已用内置规则整理；在右上角「AI 设置」填入 base_url/model/key 可让 AI 整理并解释每个接口")
	}

	res.Candidates = cands
	res.ElapsedMs = float64(time.Since(start).Microseconds()) / 1000.0
	return res
}

/* ------------------------------ 各种扫描 ------------------------------ */

func fetch(ctx context.Context, client *http.Client, u string) ([]byte, int, float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, 0, "", err
	}
	req.Header.Set("User-Agent", "kefu-loadtest/1.0 (discover)")
	start := time.Now()
	resp, err := client.Do(req)
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return nil, 0, ms, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return body, resp.StatusCode, ms, resp.Header.Get("Content-Type"), nil
}

var openAPIPaths = []string{
	"/openapi.json", "/swagger.json", "/v2/api-docs", "/api-docs",
	"/swagger/v1/swagger.json", "/docs/openapi.json", "/api/openapi.json",
}

func scanOpenAPI(ctx context.Context, client *http.Client, target string) ([]Candidate, []Fact) {
	var facts []Fact
	var cands []Candidate
	for _, p := range openAPIPaths {
		body, status, _, ct, err := fetch(ctx, client, target+p)
		if err != nil || status != http.StatusOK {
			continue
		}
		if !strings.Contains(ct, "json") && !json.Valid(body) {
			continue
		}
		var doc struct {
			Info struct {
				Title string `json:"title"`
			} `json:"info"`
			Security []map[string][]string `json:"security"`
			Paths    map[string]map[string]struct {
				Summary      string   `json:"summary"`
				OperationID  string   `json:"operationId"`
				Tags         []string `json:"tags"`
				Security     []map[string][]string `json:"security"`
				Parameters   []struct {
					Name string `json:"name"`
					In   string `json:"in"`
				} `json:"parameters"`
			} `json:"paths"`
		}
		if err := json.Unmarshal(body, &doc); err != nil || len(doc.Paths) == 0 {
			continue
		}
		needAuth := len(doc.Security) > 0
		facts = append(facts, Fact{
			Kind:   "OpenAPI",
			Detail: fmt.Sprintf("在 %s 找到接口契约：%s，共 %d 条路径（%s）", p, doc.Info.Title, len(doc.Paths), authText(needAuth)),
			Level:  "ok",
		})
		for path, ops := range doc.Paths {
			for method, op := range ops {
				m := strings.ToUpper(method)
				switch m {
				case "GET", "POST", "PUT", "PATCH", "DELETE":
				default:
					continue
				}
				name := strings.TrimSpace(op.Summary)
				if name == "" {
					name = strings.Join(op.Tags, "/") + " " + m + " " + path
					name = strings.TrimSpace(name)
				}
				risk := config.RiskRead
				if m != "GET" {
					if m == "DELETE" {
						risk = config.RiskDanger
					} else {
						risk = config.RiskWrite
					}
				}
				auth := config.AuthNone
				if needAuth || len(op.Security) > 0 {
					auth = config.AuthBearer
				}
				concrete := concretePath(path)
				cands = append(cands, Candidate{
					Scenario: config.Scenario{
						ID:          "sc_" + strings.ToLower(m) + "_" + sanitize(path),
						Name:        name,
						Method:      m,
						Path:        concrete,
						Concurrency: 5,
						IntervalMs:  300,
						JitterMs:    100,
						Risk:        risk,
						Note:        "来自 OpenAPI 契约",
					},
					Recommended: m == "GET" && auth == config.AuthNone,
					Reason:      pickReason(m, auth),
				})
			}
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].Path < cands[j].Path })
		break
	}
	return cands, facts
}

func authText(needAuth bool) string {
	if needAuth {
		return "需要鉴权，压测前要填 Token"
	}
	return "无需鉴权"
}

func pickReason(method, auth string) string {
	switch {
	case auth == config.AuthBearer:
		return "该接口需要登录 token，勾选前请先在下方填写 Token"
	case method == "GET":
		return "只读接口，压测最安全"
	case method == "DELETE":
		return "会删数据，默认不勾选"
	default:
		return "写入接口，会在库里产生真实数据"
	}
}

// concretePath 把 /users/{id} 这类路径参数换成真实值，便于直接压。
func concretePath(p string) string {
	re := regexp.MustCompile(`\{[^}]+\}`)
	return re.ReplaceAllString(p, "1")
}

func sanitize(p string) string {
	out := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, strings.Trim(p, "/"))
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// scanGuest 识别「客服系统访客链路」这一已知模式。
// uid 传空则现场生成一个；渠道识别失败时仍会把候选接口列出来（只是标记不推荐）。
func scanGuest(ctx context.Context, client *http.Client, target, channel, uid string, concurrency int) ([]Candidate, []Fact) {
	var facts []Fact
	if strings.TrimSpace(uid) == "" {
		uid = fmt.Sprintf("discover_%d", time.Now().Unix()%1000000)
	}
	initURL := fmt.Sprintf("%s/api/guest/init?channel=%s&uid=%s&nickname=%s",
		target, url.QueryEscape(channel), url.QueryEscape(uid), url.QueryEscape("勘察探测"))
	body, status, ms, _, err := fetch(ctx, client, initURL)
	if err != nil || status != http.StatusOK {
		facts = append(facts, Fact{Kind: "访客链路", Detail: "未识别到客服系统访客接口（/api/guest/init 不可用）", Level: "info"})
		return nil, facts
	}
	var initResp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Conversation struct {
				ID     int64  `json:"id"`
				Status string `json:"status"`
			} `json:"conversation"`
			Greeting string `json:"greeting"`
			Channel  struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"channel"`
		} `json:"data"`
	}
	initOK := true
	if err := json.Unmarshal(body, &initResp); err != nil || initResp.Code != 0 {
		initOK = false
		facts = append(facts, Fact{
			Kind: "访客链路",
			Detail: fmt.Sprintf("/api/guest/init 有响应但返回异常：%s（渠道「%s」可能不存在或未启用）",
				firstNonEmpty(initResp.Msg, snippetStr(body)), channel),
			Level: "warn",
		})
	} else {
		facts = append(facts, Fact{
			Kind: "访客链路",
			Detail: fmt.Sprintf("已识别客服系统访客接口：渠道 %s（%s），初始化成功（会话 #%d，%s，%.0fms）",
				initResp.Data.Channel.Name, initResp.Data.Channel.Type,
				initResp.Data.Conversation.ID, initResp.Data.Conversation.Status, ms),
			Level: "ok",
		})
		if initResp.Data.Greeting != "" {
			facts = append(facts, Fact{Kind: "访客链路", Detail: "欢迎语：" + initResp.Data.Greeting, Level: "info"})
		}
	}

	cands := []Candidate{
		{
			Scenario: config.Scenario{
				ID:          "sc_guest_message",
				Name:        "访客发消息",
				Method:      "POST",
				Path:        "/api/guest/message?channel={{channel}}&uid={{uid}}",
				Body:        `{"type":"text","content":"压测消息 {{seq}}，时间 {{ts}}"}`,
				Setup:       config.SetupGuestInit,
				Concurrency: concurrency,
				IntervalMs:  500,
				JitterMs:    150,
				Risk:        config.RiskWrite,
				Note:        "核心链路：客户发消息 → 落库 → 推送客服，最能反映接待能力",
			},
			Recommended: true,
			Reason:      "最重要的业务链路，建议必压",
		},
		{
			Scenario: config.Scenario{
				ID:          "sc_guest_init",
				Name:        "访客打开聊天页（初始化）",
				Method:      "GET",
				Path:        "/api/guest/init?channel={{channel}}&uid={{uid}}&nickname={{nick}}",
				Concurrency: 3,
				IntervalMs:  2000,
				JitterMs:    500,
				Risk:        config.RiskRead,
				Note:        "只读：模拟用户打开页面。发消息场景已自带一次初始化，一般不必单独压",
			},
			Recommended: false,
			Reason:      "已被发消息场景覆盖，单独压意义不大",
		},
		{
			Scenario: config.Scenario{
				ID:          "sc_guest_poll",
				Name:        "访客拉取消息（轮询兜底）",
				Method:      "GET",
				Path:        "/api/guest/messages?channel={{channel}}&uid={{uid}}&since_id=0",
				Setup:       config.SetupGuestInit,
				Concurrency: 5,
				IntervalMs:  1000,
				JitterMs:    200,
				Risk:        config.RiskRead,
				Note:        "只读：验证消息查询性能，会话消息越多越吃索引",
			},
			Recommended: false,
			Reason:      "只读查询，可作为第二梯队验证",
		},
		{
			Scenario: config.Scenario{
				ID:          "sc_login",
				Name:        "客服/管理员登录",
				Method:      "POST",
				Path:        "/api/auth/login",
				Body:        `{"username":"admin","password":"admin123"}`,
				Concurrency: 2,
				IntervalMs:  5000,
				JitterMs:    1000,
				Risk:        config.RiskWrite,
				Note:        "登录会写会话/token，且可能触发风控；仅在需要验证鉴权性能时压",
			},
			Recommended: false,
			Reason:      "涉及账号密码与风控，默认不压",
		},
		{
			Scenario: config.Scenario{
				ID:          "sc_stats",
				Name:        "数据统计概览",
				Method:      "GET",
				Path:        "/api/stats/overview?days=7",
				Auth:        config.AuthBearer,
				Concurrency: 3,
				IntervalMs:  1000,
				Risk:        config.RiskRead,
				Note:        "需要管理员 token；统计类接口通常最慢，值得单独观察",
			},
			Recommended: false,
			Reason:      "需要登录 token，勾选前请填写 Token",
		},
	}
	if !initOK {
		// 接口存在但渠道不对：把候选列出来（不推荐直接压），并提示改渠道
		for i := range cands {
			cands[i].Recommended = false
			if cands[i].ID == "sc_guest_message" {
				cands[i].Reason = "渠道名没匹配上，先改对渠道再压"
			}
		}
		facts = append(facts, Fact{
			Kind:   "访客链路",
			Detail: "接口路径是通的，但渠道没匹配上：把「访客渠道」改成后台里真实的渠道标识（或渠道 ID，如 kefu_xxxx）后重新勘察",
			Level:  "warn",
		})
	}
	return cands, facts
}

// scanFrontend 扫描页面与其加载的脚本（含 SPA 懒加载 chunk），解析出前端真实调用的接口。
//
// 很多前端（axios 之类）把接口写成 `.get('/guest/init')`，再靠 baseURL 拼 `/api`，
// 页面上根本没有 "/api/..." 字面量，所以这里三件事一起做：
//  1. 抓页面 HTML 与它引用的脚本，并顺着 import("./Xxx.js") 把懒加载 chunk 也扫一遍；
//  2. 收集 "/api/..." 字面量；
//  3. 识别 axios 风格的 `.(get|post|...)( '/path' )` 调用，并用 baseURL 补全前缀。
func scanFrontend(ctx context.Context, client *http.Client, target string, pages []string) ([]Candidate, []Fact) {
	var facts []Fact

	type call struct {
		method string
		path   string
	}
	seenPath := map[string]bool{}
	seenCall := map[string]bool{}
	seenURL := map[string]bool{}
	literalPaths := make([]string, 0, 24)
	calls := make([]call, 0, 24)
	basePrefix := ""
	pagesScanned, filesScanned := 0, 0

	collectText := func(text string) {
		for _, m := range apiPathRe.FindAllString(text, -1) {
			p := strings.TrimRight(m, "\"'`),.;")
			if strings.Count(p, "/") < 2 || seenPath[p] || staticExtRe.MatchString(p) {
				continue
			}
			seenPath[p] = true
			literalPaths = append(literalPaths, p)
		}
		if basePrefix == "" {
			if m := baseURLRe.FindStringSubmatch(text); m != nil {
				basePrefix = strings.TrimRight(strings.TrimSpace(m[1]), "/")
			}
		}
		for _, m := range axiosCallRe.FindAllStringSubmatch(text, -1) {
			method := strings.ToUpper(m[1])
			p := strings.TrimRight(m[2], "\"'`),.;")
			if staticExtRe.MatchString(p) {
				continue
			}
			key := method + " " + p
			if seenCall[key] {
				continue
			}
			seenCall[key] = true
			calls = append(calls, call{method: method, path: p})
		}
	}

	queue := make([]string, 0, 24)
	push := func(ref, from string) {
		u := resolveURL(from, target, ref)
		if u == "" || seenURL[u] || len(queue) >= 40 {
			return
		}
		seenURL[u] = true
		queue = append(queue, u)
	}

	// 1) 页面：抓 HTML，收集字面量，并把脚本入队
	for _, pageURL := range pages {
		if pageURL == "" || pagesScanned >= 2 {
			continue
		}
		html, status, _, _, err := fetch(ctx, client, pageURL)
		if err != nil || status != http.StatusOK {
			continue
		}
		pagesScanned++
		text := string(html)
		collectText(text)
		for _, src := range scriptSrcRe.FindAllStringSubmatch(text, 8) {
			push(src[1], pageURL)
		}
	}

	// 2) 脚本队列：顺着懒加载 chunk 继续扫
	for len(queue) > 0 && filesScanned < 16 {
		u := queue[0]
		queue = queue[1:]
		body, status, _, _, err := fetch(ctx, client, u)
		if err != nil || status != http.StatusOK {
			continue
		}
		filesScanned++
		text := string(body)
		collectText(text)
		for _, m := range chunkRefRe.FindAllStringSubmatch(text, 40) {
			push(m[1], u)
		}
	}

	if len(literalPaths) == 0 && len(calls) == 0 {
		facts = append(facts, Fact{
			Kind: "页面扫描",
			Detail: fmt.Sprintf("扫了 %d 个页面、%d 个脚本，没解析出接口调用（可能路径是动态拼接，或页面需要登录）",
				pagesScanned, filesScanned),
			Level: "info",
		})
		return nil, facts
	}

	prefix := basePrefix
	if prefix == "" {
		prefix = "/api"
	}

	cands := make([]Candidate, 0, len(literalPaths)+len(calls))
	addCand := func(method, path, note string) {
		cands = append(cands, Candidate{
			Scenario: config.Scenario{
				ID:          "sc_scan_" + sanitize(method+"_"+path),
				Name:        "页面调用接口",
				Method:      method,
				Path:        path,
				Concurrency: 3,
				IntervalMs:  1000,
				Risk:        riskOfMethod(method),
				Note:        note,
			},
			Recommended: false,
			Reason:      "从页面代码解析出来的，参数可能不全；建议先「试跑」验证",
		})
	}
	for _, p := range literalPaths {
		addCand("GET", p, "前端代码里出现的接口路径")
	}
	for _, c := range calls {
		full := c.path
		if !strings.HasPrefix(full, "/api/") && !strings.HasPrefix(full, prefix+"/") {
			full = prefix + full
		}
		addCand(c.method, full, fmt.Sprintf("前端接口调用（baseURL=%s）", prefix))
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Path < cands[j].Path })
	if len(cands) > 30 {
		cands = cands[:30]
	}

	example := make([]string, 0, 8)
	for _, c := range cands {
		if len(example) >= 8 {
			break
		}
		example = append(example, c.Method+" "+c.Path)
	}
	facts = append(facts, Fact{
		Kind: "页面扫描",
		Detail: fmt.Sprintf("扫描 %d 个页面、%d 个脚本（含懒加载 chunk，baseURL=%s），解析出 %d 个接口调用：%s",
			pagesScanned, filesScanned, prefix, len(cands), strings.Join(example, "、")),
		Level: "info",
	})
	return cands, facts
}

func riskOfMethod(method string) string {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return config.RiskRead
	case "DELETE":
		return config.RiskDanger
	default:
		return config.RiskWrite
	}
}

// resolveURL 把脚本/资源地址补成绝对地址，并只保留同源资源。
func resolveURL(from, target, ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.Trim(ref, "\"'\x60")
	if ref == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return ref
	case strings.HasPrefix(ref, "//"):
		return "https:" + ref
	}
	base, err := url.Parse(from)
	if err != nil {
		return ""
	}
	u, err := base.Parse(ref)
	if err != nil {
		return ""
	}
	// 只扫同源资源，避免把 CDN 上的东西当成目标接口
	if u.Host != base.Host {
		tu, terr := url.Parse(target)
		if terr != nil || u.Host != tu.Host {
			return ""
		}
	}
	return u.String()
}

var (
	apiPathRe   = regexp.MustCompile(`/api/[A-Za-z0-9_\-/{}$:.]{2,80}`)
	scriptSrcRe = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	chunkRefRe  = regexp.MustCompile("[\"'\x60]\\.?/?([A-Za-z0-9_\\-]+\\.js)[\"'\x60]")
	axiosCallRe = regexp.MustCompile("\\.(get|post|put|patch|delete)\\(\\s*[\"'\x60](/[A-Za-z0-9_\\-/{}$:.]{2,80})[\"'\x60]")
	baseURLRe   = regexp.MustCompile("baseURL\\s*:\\s*[\"'\x60]([^\"'\x60]+)[\"'\x60]")
	staticExtRe = regexp.MustCompile(`\.(js|css|png|jpg|jpeg|gif|svg|ico|woff2?|ttf|map|webp|mp3|html)$`)
)

func mergeCandidates(groups ...[]Candidate) []Candidate {
	out := make([]Candidate, 0, 32)
	byPath := map[string]int{}
	for _, g := range groups {
		for _, c := range g {
			key := c.Method + " " + pathKey(c.Path)
			if idx, ok := byPath[key]; ok {
				// 已有同路径候选：只补充空的说明，不覆盖已有推荐结论
				if out[idx].Note == "" {
					out[idx].Note = c.Note
				}
				continue
			}
			byPath[key] = len(out)
			out = append(out, c)
		}
	}
	return out
}

// pathKey 用于去重：忽略 query，避免「/api/x?a=1」和「/api/x」重复出现。
func pathKey(p string) string {
	if i := strings.Index(p, "?"); i >= 0 {
		return p[:i]
	}
	return p
}

/* ------------------------------ AI 整理 ------------------------------ */

const discoverSystemPrompt = `你是资深性能测试架构师。用户在压测一个 HTTP 系统，请你根据「勘察事实」和「机器勘察出的候选接口」，整理出一份可执行的压测接口清单。

要求：
1. 只增删改候选清单，不要凭空发明候选里不存在的路径；
2. 每个接口给中文名（说清业务含义）、风险等级（只读/写入/危险）、建议并发（1-200）、建议间隔（50-5000ms）、
   是否需要先初始化会话（setup: guest_init 或 none）、是否需要登录态（auth: bearer 或 none）、
   请求体模板（JSON 字符串，可为空）、是否推荐勾选（recommended）；
3. reason 用一句中文说明「为什么推荐/不推荐压它」，note 写清压测注意事项；
4. 路径里的占位符只能用 {{uid}} {{channel}} {{seq}} {{ts}} {{rand6}} {{nick}}；
5. 严格输出 JSON，不要解释文字：
{"candidates":[{"id":"","name":"","method":"","path":"","body":"","setup":"none|guest_init","auth":"none|bearer","concurrency":10,"interval_ms":500,"jitter_ms":150,"risk":"只读|写入|危险","recommended":true,"reason":"","note":""}],"hints":["给用户的中文提示，1-4 条"]}`

func refineWithAI(ctx context.Context, ai *aiclient.Client, res Result, cands []Candidate) ([]Candidate, []string, error) {
	factsJSON, _ := json.Marshal(res.Facts)
	candsJSON, _ := json.Marshal(cands)
	pageJSON, _ := json.Marshal(res.Page)
	user := fmt.Sprintf("目标接口基址：%s\n访客渠道：%s\n地址解析：%s\n\n勘察事实：\n%s\n\n候选接口：\n%s\n\n请整理输出 JSON。",
		res.Target, res.Channel, string(pageJSON), string(factsJSON), string(candsJSON))

	ctx2, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	out, err := ai.Chat(ctx2, discoverSystemPrompt, user)
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		Candidates []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Method      string `json:"method"`
			Path        string `json:"path"`
			Body        string `json:"body"`
			Setup       string `json:"setup"`
			Auth        string `json:"auth"`
			Concurrency int    `json:"concurrency"`
			IntervalMs  int    `json:"interval_ms"`
			JitterMs    int    `json:"jitter_ms"`
			Risk        string `json:"risk"`
			Recommended bool   `json:"recommended"`
			Reason      string `json:"reason"`
			Note        string `json:"note"`
		} `json:"candidates"`
		Hints []string `json:"hints"`
	}
	if err := aiclient.ExtractJSON(out, &parsed); err != nil {
		return nil, nil, err
	}
	if len(parsed.Candidates) == 0 {
		return nil, nil, fmt.Errorf("大模型没有给出任何接口")
	}

	known := map[string]Candidate{}
	for _, c := range cands {
		known[c.Method+" "+c.Path] = c
	}
	final := make([]Candidate, 0, len(parsed.Candidates))
	for i, c := range parsed.Candidates {
		sc := config.Scenario{
			ID:          strings.TrimSpace(c.ID),
			Name:        strings.TrimSpace(c.Name),
			Method:      strings.ToUpper(strings.TrimSpace(c.Method)),
			Path:        strings.TrimSpace(c.Path),
			Body:        strings.TrimSpace(c.Body),
			Setup:       strings.TrimSpace(c.Setup),
			Auth:        strings.TrimSpace(c.Auth),
			Concurrency: c.Concurrency,
			IntervalMs:  c.IntervalMs,
			JitterMs:    c.JitterMs,
			Risk:        strings.TrimSpace(c.Risk),
			Note:        strings.TrimSpace(c.Note),
		}
		if sc.Method == "" {
			sc.Method = "GET"
		}
		if !strings.HasPrefix(sc.Path, "/") && !strings.HasPrefix(sc.Path, "http") {
			continue // 路径不合法直接丢弃，避免压错地址
		}
		if sc.ID == "" {
			sc.ID = fmt.Sprintf("ai_sc_%d", i+1)
		}
		if sc.Concurrency < 1 {
			sc.Concurrency = 5
		}
		if sc.Concurrency > 500 {
			sc.Concurrency = 500
		}
		if sc.IntervalMs < 0 {
			sc.IntervalMs = 0
		}
		if sc.JitterMs < 0 {
			sc.JitterMs = 0
		}
		switch sc.Setup {
		case config.SetupGuestInit, config.SetupNone:
		default:
			sc.Setup = config.SetupNone
		}
		switch sc.Auth {
		case config.AuthBearer, config.AuthNone:
		default:
			sc.Auth = config.AuthNone
		}
		switch sc.Risk {
		case config.RiskRead, config.RiskWrite, config.RiskDanger, config.RiskUnknown:
		default:
			sc.Risk = config.RiskUnknown
		}
		// 机器勘察里有同一路径的，沿用它的初始化/鉴权/请求体，避免 AI 漏掉关键步骤
		if old, ok := known[sc.Method+" "+sc.Path]; ok {
			if sc.Setup == config.SetupNone && old.Setup == config.SetupGuestInit {
				sc.Setup = config.SetupGuestInit
			}
			if sc.Body == "" && old.Body != "" {
				sc.Body = old.Body
			}
		}
		final = append(final, Candidate{Scenario: sc, Recommended: c.Recommended, Reason: strings.TrimSpace(c.Reason)})
	}
	if len(final) == 0 {
		return nil, nil, fmt.Errorf("大模型给出的接口都不合法")
	}
	return final, parsed.Hints, nil
}

func snippetStr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
