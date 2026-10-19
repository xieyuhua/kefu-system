// Package ipinfo IP 归属地解析 SDK。
//
// 只保留无业务依赖的能力：内网 IP 判定复用 pkg/util，外部查询由调用方以 fetch
// 回调传入（HTTP 库 / 缓存 / 落库都属于业务侧实现，本包只做一次进程内缓存）。
// 因此本包不依赖应用内部包，可以独立成 SDK。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewResolver 与单例访问器 Default，
// 业务能力一律是 *Resolver 的指针接收者方法；进程内缓存是实例字段。
package ipinfo

import (
	"strings"
	"sync"

	"kefu-server/pkg/util"
)

// maxCacheEntries 进程内二级缓存上限，超出后清空重建（与改造前口径一致）。
const maxCacheEntries = 10000

// Resolver IP 归属地解析器。进程内缓存（ip → city）是它的字段。
type Resolver struct {
	mu   sync.RWMutex
	city map[string]string
}

// NewResolver 创建解析器实例（带空的进程内缓存）。
func NewResolver() *Resolver { return &Resolver{city: map[string]string{}} }

// defaultResolver 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultResolver = NewResolver()

// Default 返回包级默认实例。
func Default() *Resolver { return defaultResolver }

// LookupCity 解析 IP 归属城市，格式由 fetch 决定（如「广东 深圳」）；无法解析时返回空串。
//
//   - 空串与内网 / 保留地址直接返回空串，不调用 fetch；
//   - 命中进程内缓存时不再调用 fetch；
//   - fetch 返回空城市时不写缓存（避免把失败结果固化）；
//   - fetch 为 nil 时等同于「不解析」。
//
// 错误原样返回给调用方，由业务侧决定是否降级（改造前是静默忽略）。
func (r *Resolver) LookupCity(ip string, fetch func(ip string) (string, error)) (string, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" || util.Default().IsPrivateIP(ip) {
		return "", nil
	}

	r.mu.RLock()
	if v, ok := r.city[ip]; ok && v != "" {
		r.mu.RUnlock()
		return v, nil
	}
	r.mu.RUnlock()

	if fetch == nil {
		return "", nil
	}
	city, err := fetch(ip)
	if err != nil {
		return "", err
	}
	city = strings.TrimSpace(city)
	if city == "" {
		return "", nil
	}

	r.mu.Lock()
	if len(r.city) > maxCacheEntries {
		r.city = map[string]string{}
	}
	r.city[ip] = city
	r.mu.Unlock()
	return city, nil
}
