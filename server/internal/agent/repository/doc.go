// Package repository 客服端（agent）的自有数据访问层占位。
//
// 客服端的查询全部复用 common/repository（会话与消息、客户与标签、设备与提醒……），
// 端内没有独有的表，因此本目录只保留包说明、不存放代码。
// 若将来出现「只有客服端才查」的表，在此新增仓储；依赖方向仍为
// agent/... → common/...，不得让 common 反向 import 本包。
package repository
