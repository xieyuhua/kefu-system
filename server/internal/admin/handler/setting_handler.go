package handler

import (
	"strings"

	"kefu-server/internal/admin/service"
	"kefu-server/pkg/authctx"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"
	"kefu-server/pkg/setting"

	"github.com/gin-gonic/gin"
)

// ---------------- 系统配置（默认图片 / 提示音 / 对象存储等） ----------------
// 通用读写完全落在 platform/setting（account 的登录响应也要用），本模块只做 HTTP 适配。
// 对象存储配置（storage.object）额外的密钥脱敏 / 回填与连通性测试走 common/service 的对象存储内核。

// ListSettingsHandler 读取全部配置项
func (h *Handler) ListSettingsHandler(c *gin.Context) {
	tenantID := authctx.TenantID(c)
	list, err := setting.ListSettings(tenantID)
	if err != nil {
		response.Default().Server(c, err.Error())
		return
	}
	// 密钥类设置项必须脱敏后再返回：后端只下发 ******，回写该值表示沿用库中原密钥。
	//   - storage.object：整段 JSON 里的 secret_key；
	//   - ai.api_key：单值密钥（与对象存储同一套 ****** 口径）。
	for _, item := range list {
		key, _ := item["key"].(string)
		switch key {
		case setting.SettingObjectStorage:
			if value, ok := item["value"].(string); ok {
				item["value"] = h.svc.Storage.MaskObjectStorageSecret(value)
			}
		case setting.SettingAIAPIKey:
			if value, ok := item["value"].(string); ok {
				item["value"] = h.svc.AI.MaskAPIKey(value)
			}
		}
	}
	response.Default().OK(c, list)
}

// SaveSettingHandler 保存单个配置项
func (h *Handler) SaveSettingHandler(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	tenantID := authctx.TenantID(c)
	value := req.Value
	// 对象存储：secret_key 传 ****** 时回填库中原值，并压成紧凑 JSON、校验长度上限。
	if strings.TrimSpace(req.Key) == setting.SettingObjectStorage {
		v, err := h.svc.Storage.PrepareObjectStorageValue(tenantID, value)
		if err != nil {
			response.Default().Param(c, err.Error())
			return
		}
		value = v
	}
	// AI 密钥：传 ****** 时回填库中原值（后台表单不回显真实 Key，留空提交也不会把 Key 抹掉）。
	if strings.TrimSpace(req.Key) == setting.SettingAIAPIKey {
		value = h.svc.AI.PrepareAPIKeyValue(tenantID, value)
	}
	if err := setting.SaveSetting(tenantID, req.Key, value); err != nil {
		response.Default().Param(c, err.Error())
		return
	}
	response.Default().OK(c, true)
}

// TestStorageSettingHandler 测试对象存储配置是否可用（POST /api/settings/storage/test）。
// 请求体就是 storage.object 的 JSON（secret_key 传 ****** 表示沿用已存密钥）；
// 行为是构造客户端 → Ping（上传并删除一个探针对象）。
// 无论成功失败都返回 code=0，由 data.ok / data.message 表达结果，便于后台直接弹提示。
func (h *Handler) TestStorageSettingHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可测试对象存储配置")
		return
	}
	var req service.ObjectStorageConfig
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.TestObjectStorage(c.Request.Context(), authctx.TenantID(c), &req); err != nil {
		response.Default().OK(c, gin.H{"ok": false, "message": err.Error()})
		return
	}
	response.Default().OK(c, gin.H{"ok": true, "message": "连接成功"})
}

// TestAISettingHandler 测试 AI 接口配置是否可用（POST /api/settings/ai/test）。
// 请求体是后台表单里的 ai.* 字段（api_key 传 ****** 表示沿用已保存的密钥）；
// 行为是构造客户端 → Ping（发一条极短对话请求，验证地址 / 鉴权 / 模型名）。
// 与对象存储的测试接口同一约定：无论成功失败都返回 code=0，
// 由 data.ok / data.message 表达结果，便于后台直接弹提示。
func (h *Handler) TestAISettingHandler(c *gin.Context) {
	if !authctx.IsAdmin(c) {
		response.Default().Forbid(c, "仅管理员可测试 AI 接口配置")
		return
	}
	var req service.AITestConfig
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if err := h.svc.TestAI(c.Request.Context(), authctx.TenantID(c), &req); err != nil {
		response.Default().OK(c, gin.H{"ok": false, "message": err.Error()})
		return
	}
	response.Default().OK(c, gin.H{"ok": true, "message": "连接成功，模型可用"})
}
