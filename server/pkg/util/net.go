package util

import (
	"net"
	"strings"
)

// IsPrivateIP 判断是否为内网 / 本机 / 保留地址（这类地址没有归属地，不必查询）。
// 解析失败按内网处理，避免无意义的远程查询。
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 运营商级 NAT 段
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

// ClientIP 从请求中提取真实客户端 IP，优先级：X-Forwarded-For → X-Real-IP → RemoteAddr。
func ClientIP(remoteAddr string, headers map[string][]string) string {
	get := func(k string) string {
		if v, ok := headers[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if xff := get("X-Forwarded-For"); xff != "" {
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
