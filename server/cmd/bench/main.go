// Command bench 是客服系统的 HTTP 压测脚本（零依赖，标准库实现）。
//
// 用法：
//
//	# 1) 先启动服务（默认 8480），并准备好测试访客：
//	#    guest/init?channel=<app_id>&uid=benchv_0 ... benchv_19
//	# 2) 取管理员 token
//	$TOKEN = (Invoke-RestMethod -Uri http://127.0.0.1:8480/api/auth/login -Method Post `
//	          -ContentType 'application/json' -Body '{"account":"admin","password":"admin123"}').data.token
//	# 3) 跑压测
//	go run ./cmd/bench -token $TOKEN -ch kefu_xxxxxxxx -d 6s
//
// 输出：每个场景在 10 / 50 并发下的 RPS、p50、p90、p99 与错误数。
//
// 场景说明：
//   - 会话列表 / 统计 / 关键词搜索：读路径，衡量 N+1 与索引效果
//   - 发消息 / 访客初始化：写路径，衡量往返次数与刷盘开销
//
// 注意：
//   - 发消息场景复用 20 个固定访客（benchv_0..19），属于「热点会话」极端场景；
//     真实流量分散在不同会话时吞吐会更高。
//   - 业务错误（HTTP 200 但 code != 0）也计入错误数，避免把失败当成功统计。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// result 一个场景在一档并发下的原始结果
type result struct {
	lat []time.Duration
	err int64
}

// channelAppID 压测目标渠道的 app_id
var channelAppID string

// benchVUCount 预置的测试访客数量（benchv_0 ... benchv_{n-1}）
const benchVUCount = 20

func main() {
	base := flag.String("base", "http://127.0.0.1:8480", "服务地址")
	token := flag.String("token", "", "管理员 Bearer token（读接口必需）")
	ch := flag.String("ch", "", "渠道 app_id（写接口必需）")
	dur := flag.Duration("d", 6*time.Second, "每档并发的持续时间")
	flag.Parse()
	channelAppID = *ch

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        512,
			MaxIdleConnsPerHost: 512,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
		},
	}

	type scenario struct {
		name   string
		method string
		path   func(i int64) string
		body   []byte
	}
	scenarios := []scenario{
		{"GET  /conversations 列表", "GET", func(int64) string {
			return "/api/conversations?status=all&scope=all&page=1&page_size=20"
		}, nil},
		{"GET  /conversations/stats 统计", "GET", func(int64) string {
			return "/api/conversations/stats"
		}, nil},
		{"GET  /conversations?keyword 搜索", "GET", func(int64) string {
			return "/api/conversations?status=all&scope=all&page=1&page_size=20&keyword=b"
		}, nil},
		{"POST /guest/message 发消息", "POST", func(i int64) string {
			return fmt.Sprintf("/api/guest/message?channel=%s&uid=benchv_%d", channelAppID, i%benchVUCount)
		}, []byte(`{"type":"text","content":"bench msg"}`)},
		{"GET  /guest/init 访客初始化", "GET", func(i int64) string {
			return fmt.Sprintf("/api/guest/init?channel=%s&uid=benchv_%d", channelAppID, i%benchVUCount)
		}, nil},
	}

	fmt.Printf("目标: %s   每档时长: %s\n\n", *base, *dur)
	fmt.Printf("%-30s %5s %8s %8s %8s %8s %7s\n", "场景", "并发", "RPS", "p50", "p90", "p99", "错误")

	for _, sc := range scenarios {
		for _, conc := range []int{10, 50} {
			r := run(client, *base, *token, sc.method, sc.path, sc.body, conc, *dur)
			if len(r.lat) == 0 {
				fmt.Printf("%-30s %5d 全部失败（错误 %d）\n", sc.name, conc, r.err)
				continue
			}
			sort.Slice(r.lat, func(i, j int) bool { return r.lat[i] < r.lat[j] })
			pct := func(p float64) time.Duration { return r.lat[int(float64(len(r.lat)-1)*p)] }
			fmt.Printf("%-30s %5d %8.0f %8s %8s %8s %7d\n", sc.name, conc,
				float64(len(r.lat))/dur.Seconds(),
				ms(pct(0.5)), ms(pct(0.9)), ms(pct(0.99)), r.err)
		}
	}
}

// ms 毫秒格式化
func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

// run 以指定并发持续打一个场景，返回所有成功请求的延迟分布与错误数
func run(client *http.Client, base, token, method string, path func(int64) string, body []byte, conc int, dur time.Duration) result {
	var counter, errCount int64
	perWorker := make([][]time.Duration, conc)
	var wg sync.WaitGroup
	deadline := time.Now().Add(dur)

	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			local := make([]time.Duration, 0, 4096)
			for time.Now().Before(deadline) {
				i := atomic.AddInt64(&counter, 1)
				var rd io.Reader
				if body != nil {
					rd = bytes.NewReader(body)
				}
				req, err := http.NewRequest(method, base+path(i), rd)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					continue
				}
				raw, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				local = append(local, time.Since(start))

				// 业务错误计入错误数：HTTP 恒 200，成败看 code
				var m struct {
					Code float64 `json:"code"`
				}
				if json.Unmarshal(raw, &m) == nil && m.Code != 0 {
					atomic.AddInt64(&errCount, 1)
				}
			}
			perWorker[w] = local
		}(w)
	}
	wg.Wait()

	all := make([]time.Duration, 0, 65536)
	for _, s := range perWorker {
		all = append(all, s...)
	}
	return result{lat: all, err: errCount}
}
