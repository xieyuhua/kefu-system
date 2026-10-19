// Package idgen 生成对外可见的业务标识。
//
// 唯一性判断由调用方以闭包传入（如「channels 表里是否已有该 app_id」），
// 本包不碰数据库、不依赖应用内部包，因此可以独立成 SDK。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewGenerator 与单例访问器 Default，
// 业务能力一律是 *Generator 的指针接收者方法；调用点写 idgen.Default().GenerateAppID(exists)。
package idgen

import (
	"fmt"
	"time"

	"kefu-server/pkg/util"
)

// Generator 业务标识生成器。无状态，所有标识都是它的方法。
type Generator struct{}

// NewGenerator 创建生成器实例。
func NewGenerator() *Generator { return &Generator{} }

// defaultGenerator 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultGenerator = NewGenerator()

// Default 返回包级默认实例。
func Default() *Generator { return defaultGenerator }

// GenerateAppID 生成一个随机、可读的渠道业务标识。
// 形如 kefu_a1b2c3d4（8 位十六进制），用于区分同一类型下的不同渠道实例。
//
// exists 用于判断候选标识是否已被占用（nil 表示不做校验）。
// 调用方负责实现「查表」这一业务细节；查询失败按「未占用」处理，
// 保证一定能返回标识（与原逻辑一致）。
func (g *Generator) GenerateAppID(exists func(id string) bool) string {
	for {
		rand := util.Default().RandomHex(4)
		if rand == "" {
			break
		}
		id := "kefu_" + rand
		if exists == nil || !exists(id) {
			return id
		}
	}
	// 极端情况下退回时间戳，保证一定能生成
	return fmt.Sprintf("kefu_%d", time.Now().UnixNano())
}
