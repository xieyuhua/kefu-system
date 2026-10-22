这个目录是「自带浏览器内核」的默认位置。

把一份 Chromium 内核解压到这里，服务端会优先用它，完全不依赖系统里的 Edge/Chrome：

  browser/chrome-win64/chrome.exe      <- Chrome for Testing（推荐）
  browser/chrome.exe                   <- 或者直接放可执行文件
  browser/msedge.exe                   <- 便携版 Edge 也可以
  browser/chromium.exe

下载方式（需要能访问外网）：

  powershell -ExecutionPolicy Bypass -File ..\scripts\fetch-browser.ps1
  powershell -ExecutionPolicy Bypass -File ..\scripts\fetch-browser.ps1 -Mirror npmmirror

或者手工下载后解压：
  1) 打开 https://googlechromelabs.github.io/chrome-for-testing/
  2) 下载 win64 版 chrome-win64.zip
  3) 解压到本目录，得到 browser\chrome-win64\chrome.exe

放好之后：控制台「设置 → 自带内核目录」留空即可（默认就用这里），
然后点「诊断浏览器」确认渲染正常。

注意：自带内核使用独立用户目录 data\browser-profile，不带你系统浏览器里的登录态与插件。