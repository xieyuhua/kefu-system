package handler

// 本文件是附件上传的 HTTP 层：图片 / 视频 / 文件落盘并返回可访问 URL。
// 上传流程不涉及自有数据表（无 repository），唯一依赖是「游客态兜底租户」这一口径
// 与「对象存储是否启用」这一设置（storage.object，见 common/service）。
//
// 跨端端点：客服端 POST /api/upload（登录态）与用户端 POST /api/guest/upload
// （游客态）共用同一个处理器，故留在公共层，两端都不复制。
//
// 存储去向：
//   - 后台设置启用对象存储（S3 兼容）且连接参数齐全 → 上传到对象存储，
//     返回 public_base/key 形式的 URL；上传失败直接报错，不静默降级到本地；
//   - 未启用 / 未配置 → 保持原有本地磁盘存储（写 config.C.Upload.Dir，返回 /uploads/<day>/<file>）。
//
// 形态约定：处理器是 *UploadHandler 的方法（指针接收者），构造时建好自己的 Service；
// 只有无状态的纯工具（uploadKind / randomName / contentTypeOf）与类型白名单保留为包级定义。

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kefu-server/pkg/authctx"
	"kefu-server/pkg/config"
	"kefu-server/pkg/httpx"
	"kefu-server/pkg/response"

	"github.com/gin-gonic/gin"
	"kefu-server/internal/common/service"
)

// UploadHandler 附件上传的 HTTP 层，持有公共上传服务。
type UploadHandler struct {
	svc *service.UploadService
}

// NewUploadHandler 创建 HTTP 层实例（自带 service 依赖）。
func NewUploadHandler() *UploadHandler { return &UploadHandler{svc: service.NewUploadService()} }

// tenantIDOf 解析本次上传所属租户：登录态取 token 里的租户；
// 游客态与用户端同口径（?tenant= 优先，缺省取第一个租户）。
// 用户端的同口径实现在 client/service，公共层不能反向 import 端模块，
// 因此兜底租户复用客户内核的 CustomerService.DefaultTenantID，避免多写一份 tenants 查询。
func (h *UploadHandler) tenantIDOf(c *gin.Context) uint {
	if cl := authctx.ClaimsFromContext(c); cl != nil {
		return cl.TenantID
	}
	if t := httpx.Default().QueryUint(c, "tenant"); t > 0 {
		return t
	}
	return h.svc.DefaultTenantID()
}

// 允许上传的类型：图片 / 视频 / 文档
var (
	allowedImageExt = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true, ".bmp": true, ".svg": true,
	}
	allowedVideoExt = map[string]bool{
		".mp4": true, ".webm": true, ".mov": true, ".m4v": true, ".ogg": true,
	}
	allowedFileExt = map[string]bool{
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true, ".txt": true, ".csv": true, ".zip": true,
		".rar": true, ".7z": true, ".json": true, ".log": true,
	}
)

// uploadKind 按扩展名判定文件类别
func uploadKind(ext string) string {
	switch {
	case allowedImageExt[ext]:
		return "image"
	case allowedVideoExt[ext]:
		return "video"
	case allowedFileExt[ext]:
		return "file"
	}
	return ""
}

// contentTypeOf 取上传文件的 Content-Type：优先浏览器声明，其次按扩展名推断。
// 对象存储按该值写对象元信息，浏览器据此直接预览（图片 / 视频）。
func contentTypeOf(fileHeader *multipart.FileHeader, ext string) string {
	if fileHeader != nil && fileHeader.Header != nil {
		if ct := strings.TrimSpace(fileHeader.Header.Get("Content-Type")); ct != "" {
			return ct
		}
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// UploadHandler 上传附件（图片 / 视频 / 文件），返回可访问的 URL 与类别。
// 客服端与用户端共用；用户端走 /api/guest/upload（无需登录）。
func (h *UploadHandler) UploadHandler(c *gin.Context) {
	tenantID := h.tenantIDOf(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Default().Param(c, "请选择要上传的文件")
		return
	}
	if fileHeader.Size <= 0 {
		response.Default().Param(c, "文件内容为空")
		return
	}
	if fileHeader.Size > maxUploadSize {
		response.Default().Param(c, fmt.Sprintf("文件超过 %d MB 限制", maxUploadSize>>20))
		return
	}

	original := filepath.Base(fileHeader.Filename)
	ext := strings.ToLower(filepath.Ext(original))
	kind := uploadKind(ext)
	if kind == "" {
		response.Default().Param(c, "不支持的文件类型："+ext)
		return
	}

	// 按日期分目录，避免单目录文件过多（本地与对象存储共用 <day>/<file> 层级）
	day := time.Now().Format("20060102")
	stored := randomName(ext)

	resp := map[string]interface{}{
		"url":       "",
		"kind":      kind,
		"name":      original,
		"size":      fileHeader.Size,
		"ext":       ext,
		"tenant_id": tenantID,
	}

	// 对象存储优先：启用且连接参数齐全时上传到对象存储。
	// 上传失败直接报错，不静默降级到本地存储。
	client, ossCfg, ossErr := h.svc.ObjectStorageClient(tenantID)
	if ossErr != nil {
		response.Default().Server(c, "对象存储配置不可用："+ossErr.Error())
		return
	}
	if client != nil {
		key := ossCfg.ObjectKey(day + "/" + stored)

		src, openErr := fileHeader.Open()
		if openErr != nil {
			response.Default().Server(c, "读取上传文件失败")
			return
		}
		defer src.Close()

		url, upErr := client.Upload(c.Request.Context(), key, src, fileHeader.Size, contentTypeOf(fileHeader, ext))
		if upErr != nil {
			response.Default().Server(c, "上传到对象存储失败："+upErr.Error())
			return
		}

		log.Printf("[upload] 已上传到对象存储: %s", key)
		resp["url"] = url
		response.Default().OK(c, resp)
		return
	}

	// ---- 以下为本地磁盘存储（未启用对象存储时，行为与改造前一致） ----
	dir := filepath.Join(config.C.Upload.Dir, day)
	if err := os.MkdirAll(dir, 0755); err != nil {
		response.Default().Server(c, "创建上传目录失败")
		return
	}

	dst := filepath.Join(dir, stored)
	src, err := fileHeader.Open()
	if err != nil {
		response.Default().Server(c, "读取上传文件失败")
		return
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		response.Default().Server(c, "保存文件失败")
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		response.Default().Server(c, "写入文件失败")
		return
	}

	// 访问路径（与路由 r.Static("/uploads", ...) 对应）
	resp["url"] = "/uploads/" + day + "/" + stored
	response.Default().OK(c, resp)
}

// maxUploadSize 单文件最大 50MB（视频场景）
const maxUploadSize = 50 << 20

// randomName 生成随机文件名，避免覆盖与路径穿越
func randomName(ext string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	}
	return fmt.Sprintf("%d_%s%s", time.Now().Unix(), hex.EncodeToString(buf), ext)
}
