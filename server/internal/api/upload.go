package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kefu-server/internal/config"
	"kefu-server/internal/response"

	"github.com/gin-gonic/gin"
)

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

// Upload 上传附件（图片 / 视频 / 文件），返回可访问的 URL 与类别。
// 客服端与用户端共用；用户端走 /api/guest/upload（无需登录）。
func Upload(c *gin.Context) {
	tenantID := uint(0)
	if cl := claims(c); cl != nil {
		tenantID = cl.TenantID
	} else {
		tenantID = guestTenantID(c)
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Param(c, "请选择要上传的文件")
		return
	}
	if fileHeader.Size <= 0 {
		response.Param(c, "文件内容为空")
		return
	}
	if fileHeader.Size > maxUploadSize {
		response.Param(c, fmt.Sprintf("文件超过 %d MB 限制", maxUploadSize>>20))
		return
	}

	original := filepath.Base(fileHeader.Filename)
	ext := strings.ToLower(filepath.Ext(original))
	kind := uploadKind(ext)
	if kind == "" {
		response.Param(c, "不支持的文件类型："+ext)
		return
	}

	// 按日期分目录，避免单目录文件过多
	day := time.Now().Format("20060102")
	dir := filepath.Join(config.C.Upload.Dir, day)
	if err := os.MkdirAll(dir, 0755); err != nil {
		response.Server(c, "创建上传目录失败")
		return
	}

	stored := randomName(ext)
	dst := filepath.Join(dir, stored)
	src, err := fileHeader.Open()
	if err != nil {
		response.Server(c, "读取上传文件失败")
		return
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		response.Server(c, "保存文件失败")
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		response.Server(c, "写入文件失败")
		return
	}

	// 访问路径（与路由 r.Static("/uploads", ...) 对应）
	url := "/uploads/" + day + "/" + stored
	response.OK(c, map[string]interface{}{
		"url":       url,
		"kind":      kind,
		"name":      original,
		"size":      fileHeader.Size,
		"ext":       ext,
		"tenant_id": tenantID,
	})
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
