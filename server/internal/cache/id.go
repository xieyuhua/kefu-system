package cache

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
)

var counter uint64

// randomID 生成实例唯一 ID：时间戳 + 随机数 + 进程内自增。
func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	n := atomic.AddUint64(&counter, 1)
	return hex.EncodeToString(b) + "-" + itoa(uint(n))
}
