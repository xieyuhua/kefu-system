package cache

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options 缓存后端配置。由调用方（main）从应用配置填充，pkg 层不认识 config 包。
type Options struct {
	Enabled  bool
	Addr     string
	Password string
	DB       int
	Prefix   string
}

// Envelope 跨实例广播的信封（走 Redis Pub/Sub）。
type Envelope struct {
	TenantID  uint        `json:"tenant_id"`
	ConvID    uint        `json:"conv_id"` // >0 表示投递给用户端（访客）该会话
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
	SenderID  string      `json:"sender_id"` // 发送方实例 ID，用于忽略自己发的消息
	Timestamp int64       `json:"ts"`
}

// Driver 缓存/队列驱动：Redis 不可用时自动降级为内存实现，保证单机零依赖可跑。
type Driver interface {
	Enabled() bool
	Get(key string, out interface{}) bool
	Set(key string, val interface{}, ttl time.Duration)
	Del(keys ...string)
	Publish(ctx context.Context, topic string, env Envelope) error
	Subscribe(ctx context.Context, topic string, handler func(Envelope))
	Close() error
	Ping() error
}

var (
	drv Driver = &memoryDriver{m: map[string]item{}}
	// InstanceID 当前进程唯一标识，分布式下用于区分消息来源实例。
	InstanceID = newInstanceID()
)

func newInstanceID() string {
	return randomID()
}

// Init 初始化驱动。Redis 未启用或连接失败时静默降级。
func Init(opts Options) error {
	if !opts.Enabled {
		drv = &memoryDriver{m: map[string]item{}}
		log.Println("[cache] redis disabled, using in-memory driver (single node mode)")
		return nil
	}
	rd := &redisDriver{
		client: redis.NewClient(&redis.Options{
			Addr:     opts.Addr,
			Password: opts.Password,
			DB:       opts.DB,
		}),
		prefix: opts.Prefix,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rd.client.Ping(ctx).Err(); err != nil {
		log.Printf("[cache] WARNING redis unreachable (%v), fallback to in-memory driver", err)
		drv = &memoryDriver{m: map[string]item{}}
		return nil
	}
	drv = rd
	log.Printf("[cache] redis connected: %s (distributed mode, instance=%s)", opts.Addr, InstanceID)
	return nil
}

func Get() Driver { return drv }

// ---- key 辅助 ----

func TokenKey(token string) string   { return "token:" + token }
func BlacklistKey(jti string) string { return "jwt:blacklist:" + jti }
func OnlineKey(tenantID, userID uint) string {
	return "online:" + itoa(tenantID) + ":" + itoa(userID)
}
func ConvLockKey(convID uint) string { return "lock:conv:" + itoa(convID) }

// ---- 内存实现 ----

type item struct {
	val     []byte
	expires time.Time
}

type memoryDriver struct {
	mu sync.RWMutex
	m  map[string]item

	subMu   sync.RWMutex
	subs    map[string][]func(Envelope)
	started bool
}

func (d *memoryDriver) Enabled() bool { return false }

func (d *memoryDriver) key(k string) string { return k }

func (d *memoryDriver) Get(key string, out interface{}) bool {
	d.mu.RLock()
	it, ok := d.m[key]
	d.mu.RUnlock()
	if !ok {
		return false
	}
	if !it.expires.IsZero() && time.Now().After(it.expires) {
		d.Del(key)
		return false
	}
	if out == nil {
		return true
	}
	return json.Unmarshal(it.val, out) == nil
}

func (d *memoryDriver) Set(key string, val interface{}, ttl time.Duration) {
	b, err := json.Marshal(val)
	if err != nil {
		return
	}
	it := item{val: b}
	if ttl > 0 {
		it.expires = time.Now().Add(ttl)
	}
	d.mu.Lock()
	d.m[key] = it
	d.mu.Unlock()
}

func (d *memoryDriver) Del(keys ...string) {
	d.mu.Lock()
	for _, k := range keys {
		delete(d.m, k)
	}
	d.mu.Unlock()
}

func (d *memoryDriver) Publish(ctx context.Context, topic string, env Envelope) error {
	d.subMu.RLock()
	var hs []func(Envelope)
	hs = append(hs, d.subs[topic]...)
	d.subMu.RUnlock()
	// 内存模式下同步投递即可（同一进程内广播）
	for _, h := range hs {
		h(env)
	}
	return nil
}

func (d *memoryDriver) Subscribe(ctx context.Context, topic string, handler func(Envelope)) {
	d.subMu.Lock()
	if d.subs == nil {
		d.subs = map[string][]func(Envelope){}
	}
	d.subs[topic] = append(d.subs[topic], handler)
	d.subMu.Unlock()
}

func (d *memoryDriver) Close() error { return nil }
func (d *memoryDriver) Ping() error  { return nil }

// ---- Redis 实现 ----

type redisDriver struct {
	client *redis.Client
	prefix string
}

func (d *redisDriver) Enabled() bool { return true }
func (d *redisDriver) k(key string) string {
	if strings.HasPrefix(key, d.prefix) {
		return key
	}
	return d.prefix + key
}

func (d *redisDriver) Get(key string, out interface{}) bool {
	b, err := d.client.Get(context.Background(), d.k(key)).Bytes()
	if err != nil {
		return false
	}
	if out == nil {
		return true
	}
	return json.Unmarshal(b, out) == nil
}

func (d *redisDriver) Set(key string, val interface{}, ttl time.Duration) {
	b, err := json.Marshal(val)
	if err != nil {
		return
	}
	d.client.Set(context.Background(), d.k(key), b, ttl)
}

func (d *redisDriver) Del(keys ...string) {
	if len(keys) == 0 {
		return
	}
	full := make([]string, 0, len(keys))
	for _, k := range keys {
		full = append(full, d.k(k))
	}
	d.client.Del(context.Background(), full...)
}

func (d *redisDriver) Publish(ctx context.Context, topic string, env Envelope) error {
	if env.Timestamp == 0 {
		env.Timestamp = time.Now().UnixMilli()
	}
	if env.SenderID == "" {
		env.SenderID = InstanceID
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return d.client.Publish(ctx, d.k("topic:"+topic), b).Err()
}

func (d *redisDriver) Subscribe(ctx context.Context, topic string, handler func(Envelope)) {
	go func() {
		sub := d.client.Subscribe(ctx, d.k("topic:"+topic))
		defer sub.Close()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var env Envelope
				if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
					continue
				}
				handler(env)
			}
		}
	}()
}

func (d *redisDriver) Close() error { return d.client.Close() }
func (d *redisDriver) Ping() error  { return d.client.Ping(context.Background()).Err() }

func itoa(u uint) string {
	if u == 0 {
		return "0"
	}
	digits := []byte{}
	for u > 0 {
		digits = append([]byte{byte('0' + u%10)}, digits...)
		u /= 10
	}
	return string(digits)
}
