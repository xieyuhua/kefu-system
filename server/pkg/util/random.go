package util

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomHex 生成 n 字节随机数的十六进制字符串（长度 2n）。
// 随机源不可用时返回空串，由调用方决定降级策略。
func (u *Util) RandomHex(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
