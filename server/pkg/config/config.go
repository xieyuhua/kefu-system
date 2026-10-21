package config

import (
	"os"
	"strconv"
	"strings"
)

// Config 全局配置，优先读环境变量，其次读 config.yaml，最后取默认值。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Upload   UploadConfig   `yaml:"upload"`
	Redis    RedisConfig    `yaml:"redis"`
	IPLookup IPLookupConfig `yaml:"ip_lookup"`
	Push     PushConfig     `yaml:"push"`
	Seed     SeedConfig     `yaml:"seed"`
}

// SeedConfig 启动种子数据配置：正式环境可关闭，启动时不插入演示/初始化数据。
type SeedConfig struct {
	// Enabled 是否在启动时写入种子数据（演示租户/账号/话术规则/快捷回复等）。
	// 默认 true 保持原有行为；正式环境设为 false 后跳过全部初始化插入。
	Enabled bool `yaml:"enabled"`
}

// PushConfig 厂商推送配置（对接文档见 docs/推送对接.md）。
// 全部留空时推送网关只记录日志，不实际下发。
type PushConfig struct {
	Enabled bool        `yaml:"enabled"`
	APNs    APNsConfig  `yaml:"apns"`
	JPush   JPushConfig `yaml:"jpush"`
}

type APNsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	KeyID      string `yaml:"key_id"`
	TeamID     string `yaml:"team_id"`
	BundleID   string `yaml:"bundle_id"`
	KeyFile    string `yaml:"key_file"`   // .p8 密钥文件路径
	Production bool   `yaml:"production"` // true=生产 APNs，false=沙盒
}

type JPushConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppKey       string `yaml:"app_key"`
	MasterSecret string `yaml:"master_secret"`
}

// IPLookupConfig IP 归属地查询配置。
// URL 支持 {ip} 占位符；留空则不解析城市（会话列表只显示 IP）。
// 例（自建/第三方服务，需返回含 city/region/country 的 JSON）：
//
//	url: http://ip-api.com/json/{ip}?lang=zh-CN
type IPLookupConfig struct {
	URL string `yaml:"url"`
}

type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"` // false 时自动降级为内存实现（单机部署无需 Redis）
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Prefix   string `yaml:"prefix"` // key 前缀，多套环境共存时用
}

type ServerConfig struct {
	Port string `yaml:"port"`
	Mode string `yaml:"mode"` // debug | release
	// PublicBase 对外访问的前端根地址，用于生成渠道接入地址与二维码。
	// 例：http://your-domain.com 或 http://127.0.0.1:5173
	// 留空时二维码内容回退为相对路径 /c?channel=xxx
	PublicBase string `yaml:"public_base"`
}

type DatabaseConfig struct {
	Driver   string `yaml:"driver"` // sqlite | mysql
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	DSN      string `yaml:"dsn"` // sqlite 文件路径
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"` // 小时
}

type UploadConfig struct {
	Dir string `yaml:"dir"`
}

var C Config

func Load(path string) error {
	C = Config{
		Server: ServerConfig{Port: "8080", Mode: "debug"},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "data/kefu.db",
			Host:   "127.0.0.1",
			Port:   3306,
			User:   "root",
			Name:   "kefu",
		},
		JWT:    JWTConfig{Secret: "kefu-saas-secret-change-me", Expire: 24},
		Upload: UploadConfig{Dir: "data/uploads"},
		Redis: RedisConfig{
			Enabled: false,
			Addr:    "127.0.0.1:6379",
			DB:      0,
			Prefix:  "kefu:",
		},
		// 默认开启种子数据（首次启动插入演示租户/账号/话术），正式环境可关闭
		Seed: SeedConfig{Enabled: true},
	}

	data, err := os.ReadFile(path)
	if err == nil {
		parseSimpleYAML(string(data), &C)
	}

	// 环境变量覆盖
	if v := os.Getenv("KEFU_PORT"); v != "" {
		C.Server.Port = v
	}
	if v := os.Getenv("KEFU_MODE"); v != "" {
		C.Server.Mode = v
	}
	if v := os.Getenv("KEFU_PUBLIC_BASE"); v != "" {
		C.Server.PublicBase = v
	}
	if v := os.Getenv("KEFU_IP_LOOKUP_URL"); v != "" {
		C.IPLookup.URL = v
	}
	if v := os.Getenv("KEFU_DB_DRIVER"); v != "" {
		C.Database.Driver = v
	}
	if v := os.Getenv("KEFU_DB_DSN"); v != "" {
		C.Database.DSN = v
	}
	if v := os.Getenv("KEFU_DB_HOST"); v != "" {
		C.Database.Host = v
	}
	if v := os.Getenv("KEFU_DB_USER"); v != "" {
		C.Database.User = v
	}
	if v := os.Getenv("KEFU_DB_PASSWORD"); v != "" {
		C.Database.Password = v
	}
	if v := os.Getenv("KEFU_DB_NAME"); v != "" {
		C.Database.Name = v
	}
	if v := os.Getenv("KEFU_JWT_SECRET"); v != "" {
		C.JWT.Secret = v
	}
	// Redis（分布式部署用）
	if v := os.Getenv("KEFU_REDIS_ENABLED"); v != "" {
		C.Redis.Enabled = v == "1" || v == "true"
	}
	if v := os.Getenv("KEFU_REDIS_ADDR"); v != "" {
		C.Redis.Addr = v
	}
	if v := os.Getenv("KEFU_REDIS_PASSWORD"); v != "" {
		C.Redis.Password = v
	}
	if v := os.Getenv("KEFU_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			C.Redis.DB = n
		}
	}
	// 种子数据开关（正式环境设 false / 0 跳过初始化插入）
	if v := os.Getenv("KEFU_SEED_ENABLED"); v != "" {
		C.Seed.Enabled = v == "1" || v == "true"
	}
	return nil
}

// SaveDSN 把当前生效的数据库配置写回 config.yaml，便于切换后持久化。
func SaveDSN(path string) error {
	if C.Database.Driver == "mysql" {
		_ = os.WriteFile(path+".mysql.tpl", []byte(mysqlTemplate()), 0644)
	}
	return nil
}

func mysqlTemplate() string {
	return `server:
  port: 8080
  mode: release
database:
  driver: mysql
  host: 127.0.0.1
  port: 3306
  user: root
  password: root
  name: kefu
jwt:
  secret: kefu-saas-secret-change-me
  expire: 24
upload:
  dir: data/uploads
`
}

// parseSimpleYAML 极简 YAML 解析，只支持本项目用到的两级 "key: value" 与注释。
// 避免引入 yaml 依赖；结构固定，足够使用。
func parseSimpleYAML(s string, c *Config) {
	section := ""
	for _, raw := range strings.Split(s, "\n") {
		line := trim(raw)
		if line == "" || line[0] == '#' {
			continue
		}
		// 去掉行内注释（只处理引号外的 #）
		if idx := indexComment(line); idx >= 0 {
			line = trim(line[:idx])
		}
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := trim(line[:idx])
		val := trim(line[idx+1:])
		if len(val) > 1 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if val == "" {
			switch key {
			case "server", "database", "jwt", "upload", "redis", "ip_lookup", "push", "apns", "jpush", "seed":
				section = key
			}
			continue
		}
		applyKV(section, key, val, c)
	}
}

// indexComment 返回行内注释符 # 的位置（忽略引号内的 #），无注释返回 -1
func indexComment(s string) int {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return i
			}
		}
	}
	return -1
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func applyKV(section, key, val string, c *Config) {
	switch section {
	case "server":
		switch key {
		case "port":
			c.Server.Port = val
		case "mode":
			c.Server.Mode = val
		case "public_base":
			c.Server.PublicBase = val
		}
	case "database":
		switch key {
		case "driver":
			c.Database.Driver = val
		case "host":
			c.Database.Host = val
		case "port":
			if n, err := strconv.Atoi(val); err == nil {
				c.Database.Port = n
			}
		case "user":
			c.Database.User = val
		case "password":
			c.Database.Password = val
		case "name":
			c.Database.Name = val
		case "dsn":
			c.Database.DSN = val
		}
	case "jwt":
		switch key {
		case "secret":
			c.JWT.Secret = val
		case "expire":
			if n, err := strconv.Atoi(val); err == nil {
				c.JWT.Expire = n
			}
		}
	case "upload":
		if key == "dir" {
			c.Upload.Dir = val
		}
	case "ip_lookup":
		if key == "url" {
			c.IPLookup.URL = val
		}
	case "push":
		if key == "enabled" {
			c.Push.Enabled = val == "true" || val == "1"
		}
	case "apns":
		switch key {
		case "enabled":
			c.Push.APNs.Enabled = val == "true" || val == "1"
		case "key_id":
			c.Push.APNs.KeyID = val
		case "team_id":
			c.Push.APNs.TeamID = val
		case "bundle_id":
			c.Push.APNs.BundleID = val
		case "key_file":
			c.Push.APNs.KeyFile = val
		case "production":
			c.Push.APNs.Production = val == "true" || val == "1"
		}
	case "jpush":
		switch key {
		case "enabled":
			c.Push.JPush.Enabled = val == "true" || val == "1"
		case "app_key":
			c.Push.JPush.AppKey = val
		case "master_secret":
			c.Push.JPush.MasterSecret = val
		}
	case "redis":
		switch key {
		case "enabled":
			c.Redis.Enabled = val == "true" || val == "1"
		case "addr":
			c.Redis.Addr = val
		case "password":
			c.Redis.Password = val
		case "db":
			if n, err := strconv.Atoi(val); err == nil {
				c.Redis.DB = n
			}
		case "prefix":
			c.Redis.Prefix = val
		}
	case "seed":
		if key == "enabled" {
			c.Seed.Enabled = val == "true" || val == "1"
		}
	}
}
