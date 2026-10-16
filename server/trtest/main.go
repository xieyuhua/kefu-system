package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var base = "http://127.0.0.1:" + envOr("KEFU_PORT", "8480")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func call(method, path string, body []byte, tok string) map[string]interface{} {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r, _ := http.NewRequest(method, base+path, rd)
	r.Header.Set("Content-Type", "application/json")
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(r)
	if err != nil {
		fmt.Println("ERR", err)
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	return m
}

func main() {
	admin := call("POST", "/api/auth/login", []byte(`{"account":"admin","password":"admin123"}`), "")["data"].(map[string]interface{})["token"].(string)

	// 找一个微信渠道的 app_id 与 id
	m := call("GET", "/api/channels", nil, admin)
	chID, appID := 0, ""
	for _, it := range m["data"].([]interface{}) {
		c := it.(map[string]interface{})
		if c["type"] == "wechat" {
			chID = int(c["id"].(float64))
			appID, _ = c["app_id"].(string)
		}
	}
	fmt.Printf("1) 目标渠道 #%d (%s)\n", chID, appID)

	// 把小王也绑定到微信渠道（模拟两个客服共同接待）
	call("PUT", fmt.Sprintf("/api/channels/%d/agents", chID),
		[]byte(fmt.Sprintf(`{"user_ids":[2,3]}`)), admin)
	time.Sleep(300 * time.Millisecond)

	// 确认绑定
	m = call("GET", "/api/agents", nil, admin)
	fmt.Println("\n2) 绑定后的客服渠道:")
	for _, it := range m["data"].([]interface{}) {
		a := it.(map[string]interface{})
		names := []string{}
		if cs, ok := a["channels"].([]interface{}); ok {
			for _, c := range cs {
				names = append(names, fmt.Sprintf("%v", c.(map[string]interface{})["name"]))
			}
		}
		fmt.Printf("   %v(%v): %v\n", a["name"], a["role"], names)
	}

	// 造一个微信会话并让小李接
	call("GET", "/api/guest/init?channel="+appID+"&uid=tr_test&nickname=%E8%BD%AC%E6%8E%A5%E6%B5%8B%E8%AF%95", nil, "")
	call("POST", "/api/guest/message?channel="+appID+"&uid=tr_test", []byte(`{"type":"text","content":"hi"}`), "")
	time.Sleep(400 * time.Millisecond)

	kf1 := call("POST", "/api/auth/login", []byte(`{"account":"kf01","password":"123456"}`), "")["data"].(map[string]interface{})["token"].(string)
	m = call("GET", "/api/conversations?status=all&scope=all&page_size=50", nil, kf1)
	convID := 0
	for _, it := range m["data"].(map[string]interface{})["list"].([]interface{}) {
		c := it.(map[string]interface{})
		if c["user"].(map[string]interface{})["nickname"] == "转接测试" {
			convID = int(c["id"].(float64))
		}
	}
	if convID == 0 {
		fmt.Println("✗ 未找到测试会话")
		return
	}
	call("POST", fmt.Sprintf("/api/conversations/%d/claim", convID), nil, kf1)
	fmt.Printf("\n3) 会话 #%d 已由小李接入\n", convID)

	// 小李转给小王（合法：同渠道）
	r := call("POST", fmt.Sprintf("/api/conversations/%d/transfer", convID), []byte(`{"to_agent_id":3}`), kf1)
	fmt.Printf("4) 转给小王（同渠道）→ code=%v %s\n", r["code"], okStr(r["code"] == float64(0)))

	// 小李转给管理员（合法：管理员不受限）
	r2 := call("POST", fmt.Sprintf("/api/conversations/%d/transfer", convID), []byte(`{"to_agent_id":1}`), kf1)
	fmt.Printf("5) 转给管理员 → code=%v %s\n", r2["code"], okStr(r2["code"] == float64(0)))

	// 恢复：小李接回，然后测试转给「未绑定该渠道」的账号
	call("POST", fmt.Sprintf("/api/conversations/%d/transfer", convID), []byte(`{"to_agent_id":2}`), admin)
	time.Sleep(200 * time.Millisecond)

	// 解绑小王的微信
	call("PUT", fmt.Sprintf("/api/channels/%d/agents", chID), []byte(`{"user_ids":[2]}`), admin)
	time.Sleep(300 * time.Millisecond)

	r3 := call("POST", fmt.Sprintf("/api/conversations/%d/transfer", convID), []byte(`{"to_agent_id":3}`), kf1)
	fmt.Printf("\n6) 小王已解绑微信后再转 → code=%v msg=%v %s\n",
		r3["code"], r3["message"], okStr(fmt.Sprintf("%v", r3["code"]) != "0"))

	// 恢复绑定
	call("PUT", fmt.Sprintf("/api/channels/%d/agents", chID), []byte(`{"user_ids":[2,3]}`), admin)

	fmt.Println("\nTRANSFER API DONE")
}

func okStr(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}