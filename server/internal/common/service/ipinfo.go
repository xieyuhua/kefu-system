package service

// ---------------- IP 归属地（业务侧桥接） ----------------
// 分层约定：pkg/ipinfo 只做纯解析（内网判定 + 进程内缓存 + 调 fetch），不认识配置、缓存与数据表；
// 「外部 IP 库 HTTP 调用」「Redis 缓存」「customers 表写入」属于业务，统一落在公共业务层。
//
// 调用方（会话列表 / 客户详情 / 访客接入）继续用 CityOrLookup / RecordCustomerIP，
// 行为与改造前逐字一致：内网不解析、解析结果缓存 7 天、失败静默忽略。

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"kefu-server/internal/common/model"

	"gorm.io/gorm"
	"kefu-server/pkg/cache"
	"kefu-server/pkg/config"
	"kefu-server/pkg/ipinfo"
	"kefu-server/pkg/util"
)

// ipCityCacheTTL 归属地缓存有效期（与原实现一致：7 天）
const ipCityCacheTTL = 7 * 24 * time.Hour

// ipLookupClient 外部 IP 库查询客户端，带 3s 超时（与原实现一致）
var ipLookupClient = &http.Client{Timeout: 3 * time.Second}

// ipCityKey 缓存 key：同一租户共用一份 IP→城市 结果
func ipCityKey(ip string) string { return "ipinfo:" + ip }

// CityOrLookup 读取客户城市；库里为空但有 IP 时（历史数据）实时解析一次
func (s *CustomerService) CityOrLookup(cu *model.Customer) string {
	if cu.City != "" {
		return cu.City
	}
	return s.ResolveCity(cu.IP)
}

// RecordCustomerIP 记录客户最新 IP 与归属城市（UA 不更新）。
// 城市解析带 3s 超时与缓存；查询失败静默忽略，不影响主流程。
func (s *CustomerService) RecordCustomerIP(customerID uint, ip string) {
	s.RecordCustomerTrace(customerID, ip, "")
}

// RecordCustomerTrace 记录客户最近一次访问的 IP、归属城市与 User-Agent。
// UA 为空时只更新 IP / 城市，不动 UA 列。
func (s *CustomerService) RecordCustomerTrace(customerID uint, ip, userAgent string) {
	ip = strings.TrimSpace(ip)
	userAgent = strings.TrimSpace(userAgent)
	if customerID == 0 {
		return
	}
	updates := map[string]interface{}{}
	if ip != "" && !util.Default().IsPrivateIP(ip) {
		updates["ip"] = ip
		updates["city"] = s.ResolveCity(ip)
	}
	// User-Agent 可能有意义也可能为空（如部分 WebView 不带），空则保持原值不覆盖
	if userAgent != "" {
		if len([]rune(userAgent)) > 512 {
			userAgent = string([]rune(userAgent)[:512])
		}
		updates["user_agent"] = userAgent
	}
	if len(updates) == 0 {
		return
	}
	// 活跃时间/IP/UA 属于附属信息：丢进批量器异步落库，发消息的响应不必等这次刷盘
	DefaultWriteBatcher().SubmitAsync(func(tx *gorm.DB) error {
		return tx.Model(&model.Customer{}).Where("id = ?", customerID).Updates(updates).Error
	})
}

// ResolveCity 解析 IP 归属城市，格式如「广东 深圳」；无法解析时返回空串。
// 缓存（Redis）命中直接返回，否则交给 pkg/ipinfo 解析并回填缓存。
func (s *CustomerService) ResolveCity(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || util.Default().IsPrivateIP(ip) {
		return ""
	}

	// 1) 缓存（进程内二级缓存在 pkg/ipinfo 里）
	var cached string
	if cache.Default().Get().Get(ipCityKey(ip), &cached) && cached != "" {
		return cached
	}

	// 2) 查询（外部 IP 库实现作为 fetch 注入）
	city, err := ipinfo.Default().LookupCity(ip, fetchIPCity)
	if err != nil || city == "" {
		return ""
	}

	// 3) 回填缓存（7 天）
	cache.Default().Get().Set(ipCityKey(ip), city, ipCityCacheTTL)
	return city
}

// ipInfo 外部 IP 库的常见返回体（兼容本项目标准格式与通用格式）
type ipInfo struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	ISP     string `json:"isp"`
}

// fetchIPCity 调用外部 IP 库解析城市；任何失败都按「无结果」处理（与改造前一致）。
func fetchIPCity(ip string) (string, error) {
	base := config.C.IPLookup.URL
	if base == "" {
		return "", nil
	}
	// 占位符 {ip} 替换为实际 IP
	reqURL := strings.ReplaceAll(base, "{ip}", ip)
	resp, err := ipLookupClient.Get(reqURL)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil {
		return "", nil
	}

	// 兼容几种常见返回格式
	// 1) 本项目标准格式 {"city":"深圳","region":"广东","country":"中国"}
	var info ipInfo
	if err := json.Unmarshal(raw, &info); err == nil {
		return joinCity(info), nil
	}
	// 2) 通用格式 {"data":{"city":"深圳"}}
	var wrapper struct {
		Data ipInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if c := joinCity(wrapper.Data); c != "" {
			return c, nil
		}
	}
	// 3) 纯文本格式（整段就是城市名）
	text := strings.TrimSpace(string(raw))
	if text != "" && !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		if len([]rune(text)) <= 32 {
			return text, nil
		}
	}
	return "", nil
}

// joinCity 拼装「省份 城市」展示串
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
