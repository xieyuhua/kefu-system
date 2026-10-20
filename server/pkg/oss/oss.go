// Package oss 对象存储能力层：用 S3 兼容协议访问对象存储（MinIO、各云厂商 OOS/OSS 等）。
//
// 分层约定：本包只认识「连接参数 + 对象名」，不读写数据库、不解析后台设置项
// （settings 表里的 storage.object 由 common/service 翻译成这里的 Config）。
//
// 形态约定（与全仓一致）：包级只保留构造函数 NewClient 与无状态的取值函数，
// 业务能力一律是 *Client 的指针接收者方法；连接与配置是实例字段。
//
// S3 兼容实现的要点：
//   - endpoint 是不带 bucket 的 S3 端点（如 https://oos-cn.ctyunapi.cn），
//     带不带 http(s):// 都接受，由是否 https 决定 useSSL；
//   - 必须用 path-style（https://endpoint/bucket/key）：OOS 的 bucket 域名（bucket.endpoint）
//     在自有域名 / CDN 场景下并不总是可用，minio-go 的 BucketLookupPath 等价于
//     mc 的 --path-style，这里固定开启；
//   - 对外访问地址由调用方配置的 public_base（CDN 或 bucket 域名）决定，
//     留空时回退为 endpoint/bucket/key。
package oss

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config 对象存储连接配置。字段名与 settings 表里 storage.object 的 JSON 一致。
// enabled / provider 属于「是否启用」的业务判断，由业务层持有，能力层只关心连接参数。
type Config struct {
	// Endpoint S3 端点，不带 bucket（如 https://s3.example.com）。
	Endpoint string `json:"endpoint"`
	// Region 区域，多数 S3 兼容服务可留空。
	Region string `json:"region"`
	// Bucket 存储桶名。
	Bucket string `json:"bucket"`
	// AccessKey / SecretKey 访问密钥。
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	// PublicBase 对外访问域名（CDN 或 bucket 域名），拼接为 public_base/objectKey。
	PublicBase string `json:"public_base"`
	// PathPrefix 对象名前缀，默认 uploads/。
	PathPrefix string `json:"path_prefix"`
}

const (
	// DefaultPathPrefix 对象名前缀默认值：与本地存储的 /uploads 目录语义对齐。
	DefaultPathPrefix = "uploads/"
	// probeObjectPrefix 连通性探针对象的前缀，与业务对象分开，便于清理。
	probeObjectPrefix = ".kefu-probe/"
	// probeTimeout 探针超时：Ping 是后台「测试连接」按钮的同步调用，必须有上限。
	probeTimeout = 15 * time.Second
	// probeContentType 探针对象的 Content-Type。
	probeContentType = "text/plain; charset=utf-8"
)

// Client S3 兼容客户端，持有 minio 客户端与规范化后的配置。
type Client struct {
	mc     *minio.Client
	cfg    Config
	secure bool
}

// NewClient 用给定配置创建客户端。
// endpoint 可以带也可以不带 http(s)://：带 https:// 即 useSSL=true（其余按明文 http）。
func NewClient(cfg Config) (*Client, error) {
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	secure := strings.HasPrefix(strings.ToLower(cfg.Endpoint), "https://")
	mc, err := minio.New(hostOf(cfg.Endpoint), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
		// 固定 path-style：bucket 走路径而不是主机名，兼容性最好。
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化对象存储客户端失败: %w", err)
	}
	return &Client{mc: mc, cfg: cfg, secure: secure}, nil
}

// Config 返回客户端生效的配置（已规范化）。
func (c *Client) Config() Config { return c.cfg }

// Bucket 返回桶名。
func (c *Client) Bucket() string { return c.cfg.Bucket }

// Upload 上传一个对象，返回对外可访问的 URL。
// contentType 由调用方传入（本包不猜类型）；size 为负值时按流式（分片）上传。
func (c *Client) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	key = cleanKey(key)
	if key == "" {
		return "", errors.New("对象名不能为空")
	}
	if r == nil {
		return "", errors.New("上传内容为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	opts := minio.PutObjectOptions{ContentType: strings.TrimSpace(contentType)}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	if _, err := c.mc.PutObject(ctx, c.cfg.Bucket, key, r, size, opts); err != nil {
		return "", fmt.Errorf("上传对象 %s 失败: %w", key, err)
	}
	return c.URL(key), nil
}

// Delete 删除一个对象。对象不存在时 S3 视为成功（幂等）。
func (c *Client) Delete(ctx context.Context, key string) error {
	key = cleanKey(key)
	if key == "" {
		return errors.New("对象名不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.mc.RemoveObject(ctx, c.cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除对象 %s 失败: %w", key, err)
	}
	return nil
}

// Stat 查询对象元信息，用于校验对象确实写入成功。
func (c *Client) Stat(ctx context.Context, key string) (minio.ObjectInfo, error) {
	key = cleanKey(key)
	if key == "" {
		return minio.ObjectInfo{}, errors.New("对象名不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	info, err := c.mc.StatObject(ctx, c.cfg.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("读取对象 %s 元信息失败: %w", key, err)
	}
	return info, nil
}

// Ping 连通性探测：桶存在 → 上传一个探针对象 → 删除该探针对象。
// 只做这三步而不是列桶，避免给账号额外的 List 权限要求。
func (c *Client) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	exists, err := c.mc.BucketExists(ctx, c.cfg.Bucket)
	if err != nil {
		return fmt.Errorf("连接对象存储失败（endpoint=%s, bucket=%s）: %w", hostOf(c.cfg.Endpoint), c.cfg.Bucket, err)
	}
	if !exists {
		return fmt.Errorf("存储桶 %s 不存在（endpoint=%s）", c.cfg.Bucket, hostOf(c.cfg.Endpoint))
	}

	key := c.ProbeKey()
	body := []byte("kefu oss probe at " + time.Now().Format(time.RFC3339))
	if _, err := c.mc.PutObject(ctx, c.cfg.Bucket, key, bytes.NewReader(body), int64(len(body)),
		minio.PutObjectOptions{ContentType: probeContentType}); err != nil {
		return fmt.Errorf("探针对象上传失败（bucket=%s, key=%s）: %w", c.cfg.Bucket, key, err)
	}
	if err := c.mc.RemoveObject(ctx, c.cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("探针对象删除失败（bucket=%s, key=%s）: %w", c.cfg.Bucket, key, err)
	}
	return nil
}

// ProbeKey 返回本次探测用的对象名（带随机后缀，避免并发测试互相覆盖）。
func (c *Client) ProbeKey() string {
	return c.cfg.ObjectKey(probeObjectPrefix + fmt.Sprintf("%s.txt", randSuffix()))
}

// URL 返回对象的对外访问地址：优先 public_base，留空回退 endpoint/bucket/key。
func (c *Client) URL(key string) string {
	key = cleanKey(key)
	if base := c.cfg.PublicBase; base != "" {
		return base + "/" + key
	}
	return endpointURL(c.cfg.Endpoint, c.secure, c.cfg.Bucket, key)
}

// URL 按配置拼出对象的对外访问地址（不需要构造客户端的场景使用）。
func (c *Config) URL(key string) string {
	cfg := c.Normalize()
	if cfg.PublicBase != "" {
		return cfg.PublicBase + "/" + cleanKey(key)
	}
	secure := strings.HasPrefix(strings.ToLower(cfg.Endpoint), "https://")
	return endpointURL(cfg.Endpoint, secure, cfg.Bucket, cleanKey(key))
}

// ObjectKey 按 path_prefix 拼出对象名：ObjectKey("20250101/a.png") = uploads/20250101/a.png。
func (c *Config) ObjectKey(name string) string {
	prefix := c.Normalize().PathPrefix
	return prefix + cleanKey(name)
}

// Normalize 返回规范化后的配置副本：
//   - endpoint 去掉尾部斜杠，缺协议头时补 https://；
//   - bucket / 密钥去首尾空白，bucket 去首尾斜杠；
//   - public_base 去尾部斜杠；
//   - path_prefix 缺省 uploads/，并保证「无前导斜杠、有尾随斜杠」。
func (c *Config) Normalize() Config {
	out := *c
	out.Endpoint = normalizeEndpoint(out.Endpoint)
	out.Region = strings.TrimSpace(out.Region)
	out.Bucket = strings.Trim(strings.TrimSpace(out.Bucket), "/")
	out.AccessKey = strings.TrimSpace(out.AccessKey)
	out.SecretKey = strings.TrimSpace(out.SecretKey)
	out.PublicBase = strings.TrimRight(strings.TrimSpace(out.PublicBase), "/")

	prefix := strings.Trim(strings.TrimSpace(out.PathPrefix), "/")
	if prefix == "" {
		out.PathPrefix = DefaultPathPrefix
	} else {
		out.PathPrefix = prefix + "/"
	}
	return out
}

// Complete 报告连接参数是否齐全（endpoint / bucket / access_key / secret_key）。
func (c *Config) Complete() bool {
	return c.Validate() == nil
}

// Validate 校验连接参数，错误信息直接面向后台使用者（用于上传失败与「测试连接」提示）。
func (c *Config) Validate() error {
	cfg := c.Normalize()
	switch {
	case strings.TrimSpace(c.Endpoint) == "" && cfg.Endpoint == "":
		return errors.New("未配置对象存储端点（endpoint）")
	case hostOf(cfg.Endpoint) == "":
		return fmt.Errorf("对象存储端点不合法：%s", strings.TrimSpace(c.Endpoint))
	case cfg.Bucket == "":
		return errors.New("未配置对象存储桶（bucket）")
	case cfg.AccessKey == "":
		return errors.New("未配置对象存储 Access Key")
	case cfg.SecretKey == "":
		return errors.New("未配置对象存储 Secret Key")
	}
	return nil
}

// ---------------- 包内工具 ----------------

// hostOf 去掉 endpoint 的协议头，返回 minio.New 需要的 host[:port] 形式。
func hostOf(endpoint string) string {
	s := strings.TrimSpace(endpoint)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	return strings.Trim(s, "/")
}

// normalizeEndpoint 规范化端点：缺协议头时补 https://，去掉多余的尾部斜杠；
// 只有协议头没有主机时返回空串（由 Validate 报「端点不合法」）。
func normalizeEndpoint(raw string) string {
	ep := strings.TrimSpace(raw)
	if ep == "" {
		return ""
	}
	scheme := "https://"
	if i := strings.Index(ep, "://"); i >= 0 {
		scheme = strings.ToLower(strings.TrimSpace(ep[:i])) + "://"
		ep = ep[i+3:]
	}
	ep = strings.Trim(ep, "/")
	if ep == "" {
		return ""
	}
	return scheme + ep
}

// endpointURL 按 path-style 拼出 endpoint/bucket/key 形式的地址。
func endpointURL(endpoint string, secure bool, bucket, key string) string {
	scheme := "http"
	if secure {
		scheme = "https"
	}
	base := scheme + "://" + hostOf(endpoint)
	if bucket != "" {
		base += "/" + bucket
	}
	if key != "" {
		base += "/" + key
	}
	return base
}

// cleanKey 归一化对象名：统一用 / 分隔并去掉前导斜杠（对象名不能以 / 开头）。
func cleanKey(key string) string {
	return strings.TrimLeft(strings.TrimSpace(strings.ReplaceAll(key, "\\", "/")), "/")
}

// randSuffix 生成 8 字节随机十六进制串；随机源不可用时退回纳秒时间戳。
func randSuffix() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
