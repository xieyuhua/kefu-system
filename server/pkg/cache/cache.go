// Package cache 缓存 / 队列 SDK：Redis 与内存两种驱动，键名规则与跨实例广播。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 New / 单例访问器 Default，
// 业务能力一律是 *Cache 的指针接收者方法；驱动、实例标识都是实例字段。
// 单例语义：main 通过 cache.Default().Init(opts) 装配默认实例，全仓调用点
// 统一走 cache.Default().Get() / cache.Default().TokenKey(token)。
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

// Cache 缓存实例：驱动与实例标识都是字段，不再有包级可变状态。
type Cache struct {
	mu         sync.RWMutex
	drv        Driver
	instanceID string
}

// New 按配置构造缓存实例。Redis 未启用或连接失败时静默降级为内存驱动。
func New(opts Options) (*Cache, error) {
	c := &Cache{instanceID: randomID()}
	c.drv = c.newDriver(opts)
	return c, nil
}

// defaultCache 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
// 初始为内存驱动：即使 main 尚未 Init，读取也不会 panic。
var defaultCache = &Cache{
	drv:        &memoryDriver{m: map[string]item{}},
	instanceID: randomID(),
}

// Default 返回包级默认实例。
func Default() *Cache { return defaultCache }

// Init 在实例上按配置装配驱动（main 启动时调用一次；再次调用即重建驱动）。
func (c *Cache) Init(opts Options) error {
	drv := c.newDriver(opts)
	c.mu.Lock()
	c.drv = drv
	c.mu.Unlock()
	return nil
}

// Get 返回当前驱动。Redis 不可用时为内存实现。
func (c *Cache) Get() Driver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.drv
}

// InstanceID 当前实例的唯一标识，分布式下用于区分消息来源实例。
func (c *Cache) InstanceID() string { return c.instanceID }

// ---- key 辅助 ----

func (c *Cache) TokenKey(token string) string   { return "token:" + token }
func (c *Cache) BlacklistKey(jti string) string { return "jwt:blacklist:" + jti }
func (c *Cache) OnlineKey(tenantID, userID uint) string {
	return "online:" + itoa(tenantID) + ":" + itoa(userID)
}
func (c *Cache) ConvLockKey(convID uint) string { return "lock:conv:" + itoa(convID) }

// newDriver 按 opts 创建驱动；私有辅助，无包级状态，只被本包方法调用。
func (c *Cache) newDriver(opts Options) Driver {
	if !opts.Enabled {
		log.Println("[cache] redis disabled, using in-memory driver (single node mode)")
		return &memoryDriver{m: map[string]item{}}
	}
	rd := &redisDriver{
		client: redis.NewClient(&redis.Options{
			Addr:     opts.Addr,
			Password: opts.Password,
			DB:       opts.DB,
		}),
		prefix:     opts.Prefix,
		instanceID: c.instanceID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rd.client.Ping(ctx).Err(); err != nil {
		log.Printf("[cache] WARNING redis unreachable (%v), fallback to in-memory driver", err)
		return &memoryDriver{m: map[string]item{}}
	}
	log.Printf("[cache] redis connected: %s (distributed mode, instance=%s)", opts.Addr, c.instanceID)
	return rd
}

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
	client     *redis.Client
	prefix     string
	instanceID string
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
		env.SenderID = d.instanceID
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

// itoa 无符号整型转十进制串（包级私有辅助，无状态）。
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
