package seed

import (
	"log"
	"strings"

	"kefu-server/internal/platform/db"
	"kefu-server/internal/platform/idgen"
	"kefu-server/internal/platform/model"
)

// EnsureDemoAssets 幂等补齐内置演示 SVG 资源。
// 每次启动都执行：防止运维清理 data 目录后 /demo/*.svg 404，
// 再被 SPA 回退成 index.html，前端头像/商品图/订单图表现为「图片加载异常」。
func EnsureDemoAssets() {
	writeDemoAssets()
}

// BackfillChannelAppID 为历史数据补齐随机渠道标识（app_id）。
// 老渠道的 app_id 为空，接入链接无法区分同类型的多个渠道，这里启动时自动补一个。
// 幂等：已有 app_id 的渠道不会改动，因此重启不会让已分发的链接失效。
func BackfillChannelAppID() {
	var list []*model.Channel
	if err := db.DB.Find(&list).Error; err != nil {
		return
	}
	fixed := 0
	for _, c := range list {
		if strings.TrimSpace(c.AppID) != "" {
			continue
		}
		c.AppID = idgen.GenerateAppID(c.TenantID)
		if err := db.DB.Model(&model.Channel{}).
			Where("id = ?", c.ID).Update("app_id", c.AppID).Error; err != nil {
			continue
		}
		fixed++
	}
	if fixed > 0 {
		log.Printf("[seed] 已为 %d 个历史渠道生成随机标识 app_id，请重新分发接入链接", fixed)
	}
}
