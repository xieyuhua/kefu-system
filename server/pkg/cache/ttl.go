package cache

// TTL 进程内带过期时间的键值缓存。
//
// 用途：渠道、系统设置这类「几乎不变但每条消息都会读」的配置数据。
// 原来每次发消息都要读 settings / channels，跨网访问 MySQL 时每次多 1 个往返（~20ms）；
// 缓存后同一租户在 TTL 内不再产生这些查询。
// 取舍：配置变更最多延迟一个 TTL 生效（这里用 5s），对客服系统完全可接受。

import (
	"sync"
	"time"
)

type ttlEntry struct {
	val any
	exp time.Time
}

// TTL 并发安全的进程内缓存
type TTL struct {
	m sync.Map
}

// NewTTL 创建缓存
func NewTTL() *TTL { return &TTL{} }

// Get 取未过期的值
func (t *TTL) Get(key string) (any, bool) {
	if v, ok := t.m.Load(key); ok {
		e, _ := v.(ttlEntry)
		if time.Now().Before(e.exp) {
			return e.val, true
		}
		t.m.Delete(key)
	}
	return nil, false
}

// Set 写入值并按 ttl 过期
func (t *TTL) Set(key string, val any, ttl time.Duration) {
	t.m.Store(key, ttlEntry{val: val, exp: time.Now().Add(ttl)})
}

// Delete 删除一个键（配置变更后可主动失效）
func (t *TTL) Delete(key string) { t.m.Delete(key) }
