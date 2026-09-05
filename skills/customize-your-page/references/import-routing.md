# 导入已有页面：保留页面，适配托管路径

平台将 HTML 和资源放在按 release 隔离的子目录，作者页/作品页在 sandbox iframe 中加载
该 HTML。iframe 的 pathname 是资产入口路径，不是 `/`，也不是外层作者页地址。
资产服务按真实文件路径读取，不提供 SPA history fallback。登录、审核、上传协议不因此改变。

## 导入准备

- 有源码：复用项目依赖和构建流程，在源码中修正路径后重新 build。只动路由/构建配置，不改视觉。
- 只有静态 HTML/JS/CSS：直接使用产物，不要求它提供 package.json。先运行下述本地预览。
- 如果只有压缩后的 bundle 且路由不兼容，先在用户当前目录查找对应源码或 source map；
  没有可靠源码时说明问题并索要对应项目。不要用正则替换压缩 bundle，不要覆写 window.location、
  删除 NotFound、把所有路径指向首页或关闭 sandbox 来伪造兼容。原包保留。
- 原样可运行的多页 HTML、HashRouter 等不必更换路由；不要把所有导入都转成 React。

## 路由与资源是两件事

对于仅部署在 iframe 内的 React SPA，通常把现有 `BrowserRouter` 换成 `HashRouter`，或
把 `createBrowserRouter(routes, options)` 换成 `createHashRouter(routes, options)`。
保留原来的 routes、loader、组件与交互；移除仅为部署目录设置的 basename。检查普通
`<a href="/about">`、`location.assign('/about')` 等硬跳转，使其走现有路由器。
路由保存在 fragment 中，刷新仍请求实际 HTML 文件，不会请求平台不存在的 `/about`。
已有锚点导航或其他依赖 hash 的功能时，先处理冲突，不能盲目替换。页面内路由不等于平台的
`navigation.openWork` 等站内导航，后者保持原有 SDK 调用。

Vite 使用相对资源 base（`vite build --base ./` 或配置 `base: './'`）；其他构建工具使用对应
public path 配置。检查构建 HTML、动态 import、CSS URL、图片及 public 资源，避免 `/assets/...`
误指向平台域名根路径。绝对 HTTPS API/CDN 地址不改。**只改资源 base 不会修好 BrowserRouter**；
只配置 basename 也不能解决静态服务的 history 刷新。

参考：[React Router HashRouter](https://reactrouter.com/api/declarative-routers/HashRouter)、
[Vite relative base](https://vite.dev/guide/build#relative-base)。

## 本地预览

对含根级 `viceme-page.json` 的待上传目录（ZIP 先安全解压至工作目录）运行本 Skill 自带工具：

```sh
python3 <本Skill目录>/scripts/preview_import.py --root <待上传目录>
```

工具只绑定 `127.0.0.1`，自动选择空闲端口并输出本地 URL；它不读 CLI 凭据、不联网、不上传。
打开该 URL，产物在与平台相同 sandbox 配置的 iframe 中从嵌套路径加载。这里不模拟平台 SDK
数据或登录，不把本地效果当作真实平台接口验证。工具不提供 history fallback，也不把丢失的
JS/CSS 请求替换成 HTML，因而不会掩盖部署问题。用完停止该进程。

AI 自己检查首页、至少一个内部跳转、浏览器前进/后退及 iframe 文档刷新；外层作者页刷新会重新
载入入口并回到首页，但首页仍必须正常。原有多页/锚点功能也要保留。
检查实际内容与失败资源请求，而不是只认页面标题、HTTP 200、SDK READY 或截图。
这些是内部实现步骤，不让用户挑路由器、输入部署目录，也不增加一轮用户确认。

## 保存后

继续原有 preview 或入驻 upload/publish 流程，不增加新接口、新压缩格式或上传限制。
打开平台返回的准确地址，再检查相同交互；申请中用本人已登录状态访问，不等待审核通过。
资产 URL 可能含私有授权信息，日志/截图/报告必须遮蔽，不要把该 URL 写进源码或当作固定 basename。

若本地正确而托管仍失败：明确说明“已上传，但页面显示还没通过”，保留失败 release 与原包，
区分资源失败、客户端路由失败和真正的接口权限失败。只修复已定位的问题后重新展示效果；
不要自动撤申请、重登录、通过审核、循环发布或自行回滚别人的页面。
