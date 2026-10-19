// Package repository 用户端（client）的自有数据访问层占位。
//
// 用户端（访客）的查询全部复用 common/repository（渠道准入、客户定位、会话与消息……），
// 端内没有独有的表，因此本目录只保留包说明、不存放代码。
// 若将来出现「只有用户端才查」的表，在此新增仓储；依赖方向仍为
// client/... → common/...，不得让 common 反向 import 本包。
package repository
