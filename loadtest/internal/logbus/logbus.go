// Package logbus 是压测台的日志总线：环形缓冲 + 自增序号，供控制台增量拉取。
package logbus

import (
	"sync"
	"time"
)

// 日志级别
const (
	LevelInfo  = "info"
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Entry 是一条日志。
type Entry struct {
	Seq   int64  `json:"seq"`
	At    string `json:"at"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// Bus 并发安全的日志环形缓冲。
type Bus struct {
	mu   sync.Mutex
	buf  []Entry
	cap  int
	seq  int64
}

// New 创建日志总线，capacity 为保留的最大行数。
func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 3000
	}
	return &Bus{cap: capacity, buf: make([]Entry, 0, capacity)}
}

// Reset 清空日志（序号继续自增，前端游标不会错乱）。
func (b *Bus) Reset() {
	b.mu.Lock()
	b.buf = b.buf[:0]
	b.mu.Unlock()
}

// Add 追加一条日志。
func (b *Bus) Add(level string, msg string) {
	b.mu.Lock()
	b.seq++
	b.buf = append(b.buf, Entry{
		Seq:   b.seq,
		At:    time.Now().Format("15:04:05"),
		Level: level,
		Msg:   msg,
	})
	if len(b.buf) > b.cap {
		b.buf = append(b.buf[:0], b.buf[len(b.buf)-b.cap:]...)
	}
	b.mu.Unlock()
}

// Since 返回序号大于 seq 的日志，以及当前最大序号。
func (b *Bus) Since(seq int64) ([]Entry, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, 0, 64)
	for _, e := range b.buf {
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	return out, b.seq
}

// All 返回全部日志。
func (b *Bus) All() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.buf))
	copy(out, b.buf)
	return out
}
