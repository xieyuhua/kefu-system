package route_test

// HTTP 冒烟测试：用 httptest 直接打 route.Setup() 返回的路由。
//
// 说明（不改生产代码的前提下能测到什么）：
//   - /health 不碰数据库，可以完整断言；
//   - /api/* 由 middleware.JWTAuth 拦截，缺 token / 坏 token 都在进入业务 handler 之前返回，
//     因此不需要数据库连接（storage.Default().DB() 为 nil 也不会被触发）；
//   - 需要数据库的业务接口（登录、会话列表等）不在本文件的覆盖范围，它们的测试属于集成测试，
//     需要先注入 storage.SetDB，属于后续工作。
//
// 配置：route.Setup() 读取 config.C（gin 模式、上传目录等），
// 所以这里显式把模式设成 gin 的 test 模式，避免默认空串导致 gin.SetMode panic。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kefu-server/internal/common/middleware"
	"kefu-server/internal/route"
	"kefu-server/pkg/auth"
	"kefu-server/pkg/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// newTestEngine 准备好配置并返回被测路由。
func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()

	oldMode := config.C.Server.Mode
	oldSecret := config.C.JWT.Secret
	oldExpire := config.C.JWT.Expire

	// gin 只接受 debug/release/test 三种模式，空串会 panic
	config.C.Server.Mode = gin.TestMode
	if config.C.JWT.Secret == "" {
		config.C.JWT.Secret = "unit-test-secret"
	}
	if config.C.JWT.Expire <= 0 {
		config.C.JWT.Expire = 1
	}
	// 鉴权 SDK 不认识 config 包：生产由 main 注入，测试在这里注入同一份配置。
	auth.Default().Configure(config.C.JWT.Secret, time.Duration(config.C.JWT.Expire)*time.Hour)

	t.Cleanup(func() {
		config.C.Server.Mode = oldMode
		config.C.JWT.Secret = oldSecret
		config.C.JWT.Expire = oldExpire
	})

	return route.Setup()
}

// apiResult 统一响应体：code + message（/health 没有 message 字段，零值即为空串）。
type apiResult struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// doJSON 发起一次请求并解析统一响应体。
func doJSON(t *testing.T, r *gin.Engine, method, target string, header map[string]string) (*httptest.ResponseRecorder, apiResult) {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var res apiResult
	if body := w.Body.Bytes(); len(body) > 0 {
		if err := json.Unmarshal(body, &res); err != nil {
			t.Fatalf("%s %s 响应不是合法 JSON: %v, body=%q", method, target, err, w.Body.String())
		}
	}
	return w, res
}

func TestHealth(t *testing.T) {
	r := newTestEngine(t)

	w, res := doJSON(t, r, http.MethodGet, "/health", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /health 状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}
	if res.Code != 0 {
		t.Fatalf("GET /health 业务码 = %d, 期望 0（成功）, body=%s", res.Code, w.Body.String())
	}

	var data struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatalf("解析 /health data 失败: %v, data=%s", err, string(res.Data))
	}
	if !data.OK {
		t.Errorf("GET /health data.ok = false, 期望 true, data=%s", string(res.Data))
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /health Content-Type = %q, 期望 application/json", ct)
	}
	// CORS 中间件应给所有响应带上允许跨域的响应头
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, 期望 \"*\"", origin)
	}
}

func TestProtectedAPIWithoutToken(t *testing.T) {
	r := newTestEngine(t)

	tests := []struct {
		name    string
		target  string
		wantMsg string
	}{
		{
			name:    "会话列表：缺 Authorization 头",
			target:  "/api/conversations",
			wantMsg: "未登录",
		},
		{
			name:    "当前账号信息：缺 Authorization 头",
			target:  "/api/auth/me",
			wantMsg: "未登录",
		},
		{
			name:    "设备列表：缺 Authorization 头",
			target:  "/api/devices",
			wantMsg: "未登录",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, res := doJSON(t, r, http.MethodGet, tt.target, nil)

			// 项目约定：HTTP 状态码恒为 200，业务错误看响应体里的 code
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s 状态码 = %d, 期望 200（业务码在 body 里）", tt.target, w.Code)
			}
			if res.Code != 401 {
				t.Fatalf("GET %s 业务码 = %d, 期望 401, body=%s", tt.target, res.Code, w.Body.String())
			}
			if res.Message != tt.wantMsg {
				t.Errorf("GET %s 提示语 = %q, 期望 %q", tt.target, res.Message, tt.wantMsg)
			}
		})
	}
}

func TestProtectedAPIWithBadToken(t *testing.T) {
	r := newTestEngine(t)

	tests := []struct {
		name    string
		target  string
		header  map[string]string
		wantMsg string
	}{
		{
			name:    "会话列表：Bearer 后面是乱码",
			target:  "/api/conversations",
			header:  map[string]string{"Authorization": "Bearer not-a-real-token"},
			wantMsg: "登录已失效，请重新登录",
		},
		{
			name:    "会话列表：Authorization 为空字符串以外的非法格式（缺少 Bearer 前缀）",
			target:  "/api/conversations",
			header:  map[string]string{"Authorization": "abc.def.ghi"},
			wantMsg: "登录已失效，请重新登录",
		},
		{
			name:    "会话列表：query token 形式也走同一套校验",
			target:  "/api/conversations?token=not-a-real-token",
			wantMsg: "登录已失效，请重新登录",
		},
		{
			name:    "会话统计：错误 token",
			target:  "/api/conversations/stats",
			header:  map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiJ9.e30.bad"},
			wantMsg: "登录已失效，请重新登录",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, res := doJSON(t, r, http.MethodGet, tt.target, tt.header)

			if w.Code != http.StatusOK {
				t.Fatalf("GET %s 状态码 = %d, 期望 200", tt.target, w.Code)
			}
			if res.Code != 401 {
				t.Fatalf("GET %s 业务码 = %d, 期望 401, body=%s", tt.target, res.Code, w.Body.String())
			}
			if res.Message != tt.wantMsg {
				t.Errorf("GET %s 提示语 = %q, 期望 %q", tt.target, res.Message, tt.wantMsg)
			}
			// 未鉴权时不应把任何业务数据带出去
			if len(res.Data) > 0 && string(res.Data) != "null" {
				t.Errorf("GET %s 未鉴权却返回了 data=%s", tt.target, string(res.Data))
			}
		})
	}
}

// TestJWTAuthorizedPasses 用真实签发的 token 打一遍中间件，
// 证明 401 不是因为「所有 token 都过不去」，签名正确的 token 能进入下游 handler。
// 这里用独立的 gin 引擎挂一个不回表的桩 handler，避免依赖数据库。
func TestJWTAuthorizedPasses(t *testing.T) {
	oldSecret := config.C.JWT.Secret
	oldExpire := config.C.JWT.Expire
	config.C.JWT.Secret = "unit-test-secret"
	config.C.JWT.Expire = 1
	// 同 newTestEngine：JWT 密钥与有效期由调用方注入 pkg/auth。
	auth.Default().Configure(config.C.JWT.Secret, time.Duration(config.C.JWT.Expire)*time.Hour)
	t.Cleanup(func() {
		config.C.JWT.Secret = oldSecret
		config.C.JWT.Expire = oldExpire
	})

	token, err := auth.Default().GenerateToken(42, 7, "agent", "agent01")
	if err != nil {
		t.Fatalf("GenerateToken 失败: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken 返回空 token")
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var gotUserID, gotTenantID uint
	engine.GET("/ping", middleware.JWTAuth(), func(c *gin.Context) {
		claims, ok := c.Get("claims")
		if !ok {
			c.String(http.StatusUnauthorized, "no claims")
			return
		}
		if cl, ok := claims.(*auth.Claims); ok {
			gotUserID, gotTenantID = cl.UserID, cl.TenantID
		}
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})

	tests := []struct {
		name     string
		header   map[string]string
		wantCode int // 响应体里的业务码
	}{
		{
			name:     "签名正确的 token 放行",
			header:   map[string]string{"Authorization": "Bearer " + token},
			wantCode: 0,
		},
		{
			name:     "签名被篡改（尾部加字符）返回 401",
			header:   map[string]string{"Authorization": "Bearer " + token + "x"},
			wantCode: 401,
		},
		{
			name:     "用别的密钥签发的 token 返回 401",
			header:   map[string]string{"Authorization": "Bearer " + tamperedToken(t)},
			wantCode: 401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, gotTenantID = 0, 0

			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			// 项目约定：HTTP 状态码恒为 200，业务错误看响应体里的 code
			if w.Code != http.StatusOK {
				t.Fatalf("状态码 = %d, 期望 200（业务码在 body 里）, body=%s", w.Code, w.Body.String())
			}
			var res apiResult
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("响应不是合法 JSON: %v, body=%q", err, w.Body.String())
			}
			if res.Code != tt.wantCode {
				t.Fatalf("业务码 = %d, 期望 %d, body=%s", res.Code, tt.wantCode, w.Body.String())
			}
			switch tt.wantCode {
			case 0:
				if gotUserID != 42 || gotTenantID != 7 {
					t.Errorf("中间件解析出的 claims = (uid=%d, tid=%d), 期望 (42, 7)", gotUserID, gotTenantID)
				}
			default:
				if gotUserID != 0 || gotTenantID != 0 {
					t.Errorf("被拒绝的请求不应进入下游 handler，却读到了 claims (uid=%d, tid=%d)", gotUserID, gotTenantID)
				}
			}
		})
	}
}

// TestUnknownAPIPathReturns200 项目约定「HTTP 恒 200，错误看 body.code」：
// 未注册的 /api 路径也返回 HTTP 200，响应体为 {"code":404,"message":"接口不存在"}，
// 不能因为路由不存在就返回 HTTP 404（前端统一按 body.code 判断）。
func TestUnknownAPIPathReturns200(t *testing.T) {
	r := newTestEngine(t)

	targets := []string{
		"/api",
		"/api/not-exist",
		"/api/conversations/999999/unknown-action",
		"/api/devices/1/unknown",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			w, res := doJSON(t, r, http.MethodGet, target, nil)

			if w.Code != http.StatusOK {
				t.Fatalf("GET %s 状态码 = %d, 期望 %d（HTTP 恒 200，业务码在 body 里）",
					target, w.Code, http.StatusOK)
			}
			if res.Code != 404 {
				t.Errorf("GET %s 业务码 = %d, 期望 404, body=%s", target, res.Code, w.Body.String())
			}
			if res.Message != "接口不存在" {
				t.Errorf("GET %s 提示语 = %q, 期望 %q", target, res.Message, "接口不存在")
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("GET %s Content-Type = %q, 期望 application/json", target, ct)
			}
		})
	}
}

// tamperedToken 用错误的密钥签发一个「结构合法但验签失败」的 token。
// 这里直接调 jwt 签发而不用 auth.GenerateToken：后者会把 token 写进缓存，
// 而 ParseToken 的缓存分支不验签，那样就测不出签名错误了。
func tamperedToken(t *testing.T) string {
	t.Helper()

	claims := auth.Claims{
		UserID:   42,
		TenantID: 7,
		Role:     "agent",
		Account:  "agent01",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("a-different-secret"))
	if err != nil {
		t.Fatalf("用另一把密钥签发 token 失败: %v", err)
	}
	return token
}
