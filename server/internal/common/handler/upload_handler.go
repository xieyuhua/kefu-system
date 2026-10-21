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
	"errors"
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
	"kefu-server/pkg/setting"

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
	// 大小上限来自后台设置 upload.max_size（MB，默认 50）
	maxSize := setting.UploadMaxSizeBytes(tenantID)
	if fileHeader.Size > maxSize {
		response.Default().Param(c, fmt.Sprintf("文件超过 %d MB 限制", maxSize>>20))
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

// maxUploadSize 单文件最大 50MB（视频场景）。注意：实际校验已改为后台设置 upload.max_size，
// 此常量仅作为兜底与提示文案的历史参照，不再参与上传校验。
const maxUploadSize = 50 << 20

// randomName 生成随机文件名，避免覆盖与路径穿越
func randomName(ext string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	}
	return fmt.Sprintf("%d_%s%s", time.Now().Unix(), hex.EncodeToString(buf), ext)
}

// ---------------- 分片上传（断点续传） ----------------
//
// 前端把大文件切成固定大小的分片逐片上传；任一片失败只需重传该片，
// 已成功的分片落盘在 .chunks/<upload_id>/ 下，重试时跳过。全部传完后调 merge 合并。
// 合并后走与普通上传完全相同的存储分支（对象存储优先，否则本地磁盘）。

// safeUploadID 校验上传会话 ID：只允许十六进制与连字符（前端生成 UUID），防路径穿越。
func safeUploadID(raw string) bool {
	if len(raw) < 8 || len(raw) > 64 {
		return false
	}
	for _, r := range raw {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') && r != '-' {
			return false
		}
	}
	return true
}

// chunkDirOf 分片临时目录（按 upload_id 隔离）
func chunkDirOf(uploadID string) string {
	return filepath.Join(config.C.Upload.Dir, ".chunks", uploadID)
}

// UploadChunkHandler 上传单个分片（multipart file 字段：file；query：upload_id / index）。
// 每个分片独立落盘，客户端失败重试时跳过已存在的分片。
func (h *UploadHandler) UploadChunkHandler(c *gin.Context) {
	tenantID := h.tenantIDOf(c)
	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if !safeUploadID(uploadID) {
		response.Default().Param(c, "上传会话 ID 不合法")
		return
	}
	index := httpx.Default().QueryUint(c, "index")
	if index < 0 || index > 10000 {
		response.Default().Param(c, "分片序号不合法")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Default().Param(c, "缺少分片内容")
		return
	}
	if fileHeader.Size <= 0 {
		response.Default().Param(c, "分片内容为空")
		return
	}

	dir := chunkDirOf(uploadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		response.Default().Server(c, "创建分片目录失败")
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		response.Default().Server(c, "读取分片失败")
		return
	}
	defer src.Close()

	dst := filepath.Join(dir, fmt.Sprintf("%d.part", index))
	out, err := os.Create(dst)
	if err != nil {
		response.Default().Server(c, "保存分片失败")
		return
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		response.Default().Server(c, "写入分片失败")
		return
	}
	if err := out.Close(); err != nil {
		response.Default().Server(c, "写入分片失败")
		return
	}

	response.Default().OK(c, gin.H{"upload_id": uploadID, "index": index, "received": true, "tenant_id": tenantID})
}

// MergeChunksRequest 合并分片请求体
type MergeChunksRequest struct {
	UploadID string `json:"upload_id"`
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	Size     int64  `json:"size"`
	Total    int    `json:"total"`
}

// MergeChunksHandler 合并已上传的分片，生成最终文件并返回与普通上传一致的载荷。
func (h *UploadHandler) MergeChunksHandler(c *gin.Context) {
	tenantID := h.tenantIDOf(c)
	var req MergeChunksRequest
	if !httpx.Default().BindJSON(c, &req) {
		return
	}
	if !safeUploadID(req.UploadID) {
		response.Default().Param(c, "上传会话 ID 不合法")
		return
	}
	if req.Size <= 0 {
		response.Default().Param(c, "文件大小不合法")
		return
	}
	maxSize := setting.UploadMaxSizeBytes(tenantID)
	if req.Size > maxSize {
		response.Default().Param(c, fmt.Sprintf("文件超过 %d MB 限制", maxSize>>20))
		return
	}
	if req.Total <= 0 || req.Total > 10000 {
		response.Default().Param(c, "分片数量不合法")
		return
	}
	if strings.TrimSpace(req.Filename) == "" {
		response.Default().Param(c, "文件名不能为空")
		return
	}

	dir := chunkDirOf(req.UploadID)
	// 校验所有分片齐全
	var totalBytes int64
	for i := 0; i < req.Total; i++ {
		part := filepath.Join(dir, fmt.Sprintf("%d.part", i))
		st, err := os.Stat(part)
		if err != nil {
			response.Default().Param(c, fmt.Sprintf("缺少第 %d 个分片，请重传", i+1))
			return
		}
		totalBytes += st.Size()
	}
	if totalBytes != req.Size {
		response.Default().Param(c, "分片大小与文件大小不一致，请重新上传")
		return
	}

	original := filepath.Base(req.Filename)
	ext := strings.ToLower(filepath.Ext(original))
	if req.Ext != "" {
		ext = strings.ToLower(req.Ext)
	}
	kind := uploadKind(ext)
	if kind == "" {
		response.Default().Param(c, "不支持的文件类型："+ext)
		return
	}

	day := time.Now().Format("20060102")
	stored := randomName(ext)

	resp := map[string]interface{}{
		"url":       "",
		"kind":      kind,
		"name":      original,
		"size":      req.Size,
		"ext":       ext,
		"tenant_id": tenantID,
	}

	// 对象存储优先：合并到临时文件后上传
	client, ossCfg, ossErr := h.svc.ObjectStorageClient(tenantID)
	if ossErr != nil {
		response.Default().Server(c, "对象存储配置不可用："+ossErr.Error())
		return
	}

	if client != nil {
		merged, cleanup, err := mergePartsToTemp(dir, req.Total)
		if err != nil {
			response.Default().Server(c, err.Error())
			return
		}
		defer cleanup()

		f, err := os.Open(merged)
		if err != nil {
			response.Default().Server(c, "读取合并文件失败")
			return
		}
		defer f.Close()

		key := ossCfg.ObjectKey(day + "/" + stored)
		url, upErr := client.Upload(c.Request.Context(), key, f, req.Size, contentTypeOf(nil, ext))
		if upErr != nil {
			response.Default().Server(c, "上传到对象存储失败："+upErr.Error())
			return
		}
		_ = os.RemoveAll(dir)
		log.Printf("[upload] 分片合并后上传到对象存储: %s", key)
		resp["url"] = url
		response.Default().OK(c, resp)
		return
	}

	// 本地磁盘：分片直接拼到最终文件
	finalDir := filepath.Join(config.C.Upload.Dir, day)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		response.Default().Server(c, "创建上传目录失败")
		return
	}
	dst := filepath.Join(finalDir, stored)
	out, err := os.Create(dst)
	if err != nil {
		response.Default().Server(c, "保存文件失败")
		return
	}
	for i := 0; i < req.Total; i++ {
		part := filepath.Join(dir, fmt.Sprintf("%d.part", i))
		f, err := os.Open(part)
		if err != nil {
			out.Close()
			response.Default().Server(c, "读取分片失败")
			return
		}
		if _, err := io.Copy(out, f); err != nil {
			f.Close()
			out.Close()
			response.Default().Server(c, "合并分片失败")
			return
		}
		f.Close()
	}
	if err := out.Close(); err != nil {
		response.Default().Server(c, "保存文件失败")
		return
	}
	_ = os.RemoveAll(dir)

	resp["url"] = "/uploads/" + day + "/" + stored
	response.Default().OK(c, resp)
}

// mergePartsToTemp 把分片拼成一个临时文件，返回路径与清理函数。
func mergePartsToTemp(dir string, total int) (string, func(), error) {
	tmp, err := os.CreateTemp(dir, "merged-*")
	if err != nil {
		return "", nil, errors.New("创建临时文件失败")
	}
	cleanup := func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}
	for i := 0; i < total; i++ {
		part := filepath.Join(dir, fmt.Sprintf("%d.part", i))
		f, err := os.Open(part)
		if err != nil {
			cleanup()
			return "", nil, errors.New("读取分片失败")
		}
		if _, err := io.Copy(tmp, f); err != nil {
			f.Close()
			cleanup()
			return "", nil, errors.New("合并分片失败")
		}
		f.Close()
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, errors.New("合并分片失败")
	}
	return tmp.Name(), cleanup, nil
}
