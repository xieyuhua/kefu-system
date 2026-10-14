package service

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"kefu-server/internal/cache"
	"kefu-server/internal/config"
	"kefu-server/internal/model"
)

// ---------------- IP 归属地解析 ----------------
// 内网 IP 不解析；优先走「本地 IP 库 / 外部查询服务」，结果写入缓存（Redis 或内存），
// 外部服务不可用时返回空城市，不影响主流程。

type ipInfo struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	ISP     string `json:"isp"`
}

var (
	ipInfoCache = map[string]string{} // ip → city，仅作 Redis 不可用时的进程内二级缓存
	ipInfoMu    sync.RWMutex
	ipClient    = &http.Client{Timeout: 3 * time.Second}
)

// cityOrLookup 读取客户城市；库里为空但有 IP 时（历史数据）实时解析一次
func cityOrLookup(cu model.Customer) string {
	if cu.City != "" {
		return cu.City
	}
	return ResolveCity(cu.IP)
}

// RecordCustomerIP 记录客户最新 IP 与归属城市。
// 城市解析带 3s 超时与缓存；查询失败静默忽略，不影响主流程。
func RecordCustomerIP(customerID uint, ip string) {
	ip = strings.TrimSpace(ip)
	if customerID == 0 || ip == "" || isPrivateIP(ip) {
		return
	}
	gormDB().Model(&model.Customer{}).Where("id = ?", customerID).
		Updates(map[string]interface{}{"ip": ip, "city": ResolveCity(ip)})
}

// ipKey 缓存 key：同一租户共用一份 IP→城市 结果
func ipKey(ip string) string { return "ipinfo:" + ip }

// ResolveCity 解析 IP 归属城市，格式如「广东 深圳」；无法解析时返回空串。
func ResolveCity(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || isPrivateIP(ip) {
		return ""
	}

	// 1) 缓存
	var cached string
	if cache.Get().Get(ipKey(ip), &cached) && cached != "" {
		return cached
	}
	ipInfoMu.RLock()
	if v, ok := ipInfoCache[ip]; ok && v != "" {
		ipInfoMu.RUnlock()
		return v
	}
	ipInfoMu.RUnlock()

	// 2) 查询
	city := queryIPCity(ip)
	if city == "" {
		return ""
	}

	// 3) 回填缓存（7 天）
	cache.Get().Set(ipKey(ip), city, 7*24*time.Hour)
	ipInfoMu.Lock()
	if len(ipInfoCache) > 10000 {
		ipInfoCache = map[string]string{}
	}
	ipInfoCache[ip] = city
	ipInfoMu.Unlock()
	return city
}

func queryIPCity(ip string) string {
	base := config.C.IPLookup.URL
	if base == "" {
		return ""
	}
	// 占位符 {ip} 替换为实际 IP
	reqURL := strings.ReplaceAll(base, "{ip}", ip)
	resp, err := ipClient.Get(reqURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil {
		return ""
	}

	// 兼容几种常见返回格式
	// 1) 本项目标准格式 {"city":"深圳","region":"广东","country":"中国"}
	var info ipInfo
	if err := json.Unmarshal(raw, &info); err == nil {
		return joinCity(info)
	}
	// 2) 通用格式 {"data":{"city":"深圳"}}
	var wrapper struct {
		Data ipInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if c := joinCity(wrapper.Data); c != "" {
			return c
		}
	}
	// 3) 纯文本格式（整段就是城市名）
	text := strings.TrimSpace(string(raw))
	if text != "" && !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		if len([]rune(text)) <= 32 {
			return text
		}
	}
	return ""
}

func joinCity(i ipInfo) string {
	// 去掉「中国」前缀，省得显示成「中国 广东 深圳」
	region := strings.TrimSpace(i.Region)
	if region == "中国" || region == "中华人民共和国" {
		region = ""
	}
	city := strings.TrimSpace(i.City)
	// 城市可能带后缀如「深圳市」，保留原样更贴近当地叫法
	parts := make([]string, 0, 2)
	if region != "" {
		parts = append(parts, region)
	}
	if city != "" && city != region {
		parts = append(parts, city)
	}
	if len(parts) == 0 {
		return strings.TrimSpace(i.Country)
	}
	return strings.Join(parts, " ")
}

// isPrivateIP 判断是否为内网/本机/保留地址（这类地址没有归属地）
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return true // 解析失败按内网处理，避免无意义查询
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// 100.64.0.0/10 运营商级 NAT 段
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		// 192.0.2.0/24 等文档保留段
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 2 {
			return true
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
	}
	return false
}

// ClientIP 从请求中提取真实客户端 IP，按优先级：X-Forwarded-For → X-Real-IP → RemoteAddr
func ClientIP(remoteAddr string, headers map[string][]string) string {
	get := func(k string) string {
		if v, ok := headers[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if xff := get("X-Forwarded-For"); xff != "" {
		// XFF 可能是 "client, proxy1, proxy2"，取第一个
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}
