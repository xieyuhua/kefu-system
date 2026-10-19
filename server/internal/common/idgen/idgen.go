// Package idgen 生成对外可见的业务标识。
//
// GenerateAppID 原先是 channel service 的一部分，但 platform/seed 在启动时也要用它
// 给历史渠道补齐标识；放到 platform 才能让 seed 不反向依赖业务模块。
//
// 分层例外：本包直接查 channels 表做唯一性校验（不经过任何模块的 repository），
// 因为它是基础设施能力的实现细节，且必须被 seed / 渠道模块共同使用。
package idgen

import (
	"fmt"
	"time"

	"kefu-server/internal/common/model"
	"kefu-server/pkg/storage"
	"kefu-server/pkg/util"
)

// GenerateAppID 生成一个随机、可读的渠道业务标识。
// 形如 kefu_a1b2c3d4（8 位十六进制），用于区分同一类型下的不同渠道实例。
func GenerateAppID(tenantID uint) string {
	for {
		rand := util.RandomHex(4)
		if rand == "" {
			break
		}
		id := "kefu_" + rand
		// 查询失败按「未占用」处理，保证一定能返回标识（与原逻辑一致）
		var cnt int64
		_ = storage.DB().Model(&model.Channel{}).
			Where("tenant_id = ? AND app_id = ?", tenantID, id).Count(&cnt).Error
		if cnt == 0 {
			return id
		}
	}
	// 极端情况下退回时间戳，保证一定能生成
	return fmt.Sprintf("kefu_%d", time.Now().UnixNano())
}
