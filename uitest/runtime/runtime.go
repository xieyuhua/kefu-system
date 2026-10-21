// Package jsruntime 用 go:embed 内嵌浏览器侧共享运行时脚本。
//
// uitest-keys.js 是「稳定 ID 绑定 + 多级兜底 + 自愈定位」的唯一实现：
// 服务端执行器把它注入页面，浏览器插件的 content script 也用同一份文件，
// 因此录制时算出的 data-uitest-id 与执行时算出的永远一致。
package jsruntime

import _ "embed"

// KeysJS 浏览器侧共享运行时源码。
//
//go:embed uitest-keys.js
var KeysJS string
