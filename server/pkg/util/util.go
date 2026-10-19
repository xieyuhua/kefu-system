// Package util 通用工具集：时间格式化、IP 判定与解析、随机串。
//
// 形态约定（与 internal 一致）：包级只保留构造函数 NewUtil 与单例访问器 Default，
// 业务能力一律是 *Util 的指针接收者方法。本包无状态，方法不读也不写字段；
// 调用点统一写 util.Default().FmtTime(t)，避免到处 new。
package util

// Util 通用工具集。无状态，所有能力都是它的方法。
type Util struct{}

// NewUtil 创建工具集实例。
func NewUtil() *Util { return &Util{} }

// defaultUtil 进程内默认实例（语义等同构造函数，供全仓调用点共用）。
var defaultUtil = NewUtil()

// Default 返回包级默认实例。
func Default() *Util { return defaultUtil }
