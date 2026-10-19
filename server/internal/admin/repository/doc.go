// Package repository 后台端（admin）的自有数据访问层占位。
//
// 后台端的查询全部复用 common/repository（渠道、子账号、客户与标签、自动回复、统计……），
// 端内没有独有的表，因此本目录只保留包说明、不存放代码。
// 若将来出现「只有后台端才查」的表，在此新增仓储；依赖方向仍为
// admin/... → common/...，不得让 common 反向 import 本包。
package repository
