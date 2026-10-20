package route_test

// 对象存储（天翼云 OOS）相关的路由级测试：新增的 POST /api/settings/storage/test
// 必须挂在和其它 settings 接口同一组鉴权下（登录 + 管理员），且不能影响未登录时的
// 既有行为（HTTP 恒 200，业务码看 body.code）。
//
// 这里只覆盖「进入业务 handler 之前」的分支（缺 token / 非管理员），
// 因此不需要数据库连接；真正连对象存储的用例由验收脚本打真实服务端验证。

import (
	"net/http"
	"testing"

	"kefu-server/pkg/auth"
)

// TestStorageTestRouteRequiresAuth 未登录时，新增的 test 接口必须被鉴权中间件拦下，
// 而不是落到 NoRoute 的「接口不存在」（code=404）。
func TestStorageTestRouteRequiresAuth(t *testing.T) {
	r := newTestEngine(t)

	w, res := doJSON(t, r, http.MethodPost, "/api/settings/storage/test", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200（业务码在 body 里）", w.Code)
	}
	if res.Code != 401 {
		t.Fatalf("业务码 = %d, 期望 401（未登录）, body=%s", res.Code, w.Body.String())
	}
	if res.Message != "未登录" {
		t.Errorf("提示语 = %q, 期望 %q", res.Message, "未登录")
	}
}

// TestStorageTestRouteRejectsNonAdmin 已登录但不是管理员：返回 403，
// 与「管理员权限」的契约一致（其它 settings 接口本身不做角色区分，此处只多这一层）。
func TestStorageTestRouteRejectsNonAdmin(t *testing.T) {
	r := newTestEngine(t)

	token, err := auth.Default().GenerateToken(42, 7, "agent", "agent01")
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}

	w, res := doJSON(t, r, http.MethodPost, "/api/settings/storage/test",
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
	if res.Code != 403 {
		t.Fatalf("业务码 = %d, 期望 403（非管理员）, body=%s", res.Code, w.Body.String())
	}
	if res.Message == "" {
		t.Error("403 应带可读提示语")
	}
}

// TestSettingsRoutesUnchanged 原有的 settings 读写接口行为不变（未登录仍 401）。
func TestSettingsRoutesUnchanged(t *testing.T) {
	r := newTestEngine(t)

	tests := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/settings"},
		{http.MethodPut, "/api/settings"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			_, res := doJSON(t, r, tt.method, tt.target, nil)
			if res.Code != 401 {
				t.Fatalf("业务码 = %d, 期望 401, body=%s", res.Code, "")
			}
		})
	}
}
