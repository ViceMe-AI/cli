---
name: customize-your-page
description: 用 AI 创建、上传、预览、发布或更新 ViceMe 作者页和作品页。适用于用户要自定义页面 UI、把已有网页 ZIP 绑定到作者或作品、查看可调用的平台接口，或回滚历史页面版本。
---

# 自定义 ViceMe 页面

作者页和作品页共用这一套流程。页面本身由当前 Agent 使用正常的前端工具创建或修改；
ViceMe CLI 只负责确认目标、返回平台接口、校验 ZIP 结构、上传、预览、发布和回滚。
接口细节按需阅读 [runtime-api.md](references/runtime-api.md)。

面向用户的文字跟随当前语言。中文使用自然白话，只说“确认页面”“准备预览”“上传”
和“发布”等业务动作；不展示命令、权限名、账号 ID、错误码、原始 JSON 或内部状态，
也不告诉用户正在使用哪个内置 Skill。

## 固定流程

1. 取得用户要修改的准确 ViceMe 作者页或作品页 URL。入口已经给出 URL 时直接使用，
   不再让用户选择作者页还是作品页；没有 URL 时只问这一项。
2. 第一条用户可见回复是“我先检查登录、创作者资格和这个页面能用的功能。”随后以
   玩法守卫模式调用 `$become-a-creator`。只有它返回当前用户拥有的有效 Merchant 才继续；
   申请中、拒绝或用户不同意申请时停止。
3. 运行 `viceme merchant page describe --target <准确URL> --merchant <商家ID>`。
   这一步同时确认页面归属，并返回该目标当前可用的分类接口。必须以本次响应为准，
   不凭记忆假设作者页或作品页有什么接口。
4. 检查用户已有项目、网页目录或 ZIP。已有页面时保留它的技术栈、视觉和功能，只补充
   ViceMe 接口及打包文件；没有页面时根据用户要求创建。普通 HTML、CSS、JavaScript、
   多页面、外部 HTTPS API、CDN、iframe 和静态资源都可以使用，不做源码安全审计，
   也不因为疑似密钥字符串自行阻止上传。
5. 在 ZIP 根目录放置 `viceme-page.json`，声明准确的页面 kind、真实 HTML 入口，以及
   页面实际需要的平台 capabilities。不要为了省事声明所有 capabilities。ZIP 可以采用
   任意目录结构，入口不要求叫 `dist/index.html`。
6. 运行 `viceme merchant page inspect --path <zip>`。只修复它返回的包结构问题，例如
   ZIP 无法读取、路径穿越、压缩炸弹、缺少 manifest、入口不存在或 manifest 不匹配；
   不借此增加浏览器源码限制。
7. 运行 `viceme merchant page preview --path <zip> --target <准确URL> --merchant <商家ID>`，
   打开命令返回的完整预览 URL，并把同一 URL 作为 Markdown 备用链接给用户。预览期间
   根据用户反馈修改并重新预览。
8. 预览不等于公开发布。只有展示最终预览并得到用户明确确认后，才运行
   `viceme merchant page status --target <准确URL> --merchant <商家ID>`，使用这次预览的
   release ID 和响应中的当前 active release，运行
   `viceme merchant page publish <release-id> --expected-active <当前ID或none> --merchant <商家ID>`。
9. 发布后把第 1 步确认的准确目标 URL 交给用户，不自行重建地址。后续更新重复 describe、制作、inspect、
   preview、确认和 publish；用户明确要求回滚时，先读 status，再对准确历史 release 运行
   `viceme merchant page activate`，并传当前 active release。

## 不变量

- 只通过 `viceme` 访问 ViceMe，不读取配置文件、Token、Cookie 或本地凭证。
- 登录、归属、评论、点赞、购买和订阅权限由宿主接口逐项判断；页面源码不是权限边界。
- 不自行拼接预览和公开链接，不把一个页面的 release、预览 Token 或商家 ID 用到另一个目标。
- `describe` 返回 `access: USER` 的调用可能要求访客登录。页面应捕获失败，并在需要时调用
  `window.viceme.auth.requestLogin()`，不能把“按钮能显示”当成“访客已获授权”。
- 页面包通过结构检查后就允许上传。浏览器本身的混合内容、CORS 或第三方服务限制可以在
  预览中修正，但不升级成 ViceMe 上传禁令。
