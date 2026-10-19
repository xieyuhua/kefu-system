// Package handler 三端共用的 HTTP 处理器：账号（登录 / 登出 / 当前用户 / 改资料 / 改密码）、
// 附件上传（登录态与游客态）、订单与商品。
//
// 这些端点三端都调、语义完全一致（同一套口令与 token 口径、同一套分页参数），
// 复制到任一端都会立刻出现第二份实现，因此与它们的路由（internal/common/route）一起下沉到公共层。
//
// 依赖方向：common/handler → common/service。handler 层不持有数据库连接：
// 参数绑定 → 调公共 service → 写响应。
//
// 形态约定：处理器是 *AuthHandler / *UploadHandler / *CustomerHandler 的方法（指针接收者），
// Handler 在构造时建好自己的 Service；只有无状态的纯工具（authFail / uploadKind / randomName）
// 与类型白名单保留为包级定义。
package handler
