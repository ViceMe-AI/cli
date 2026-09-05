---
name: customize-your-page
description: 用 AI 创建、上传、预览、发布或更新 ViceMe 作者页和作品页。适用于用户要自定义页面 UI、把已有网页 ZIP 绑定到作者或作品、查看可调用的平台接口，或回滚历史页面版本。
---

# 自定义 ViceMe 页面

首次调用 CLI 或判断 CLI 不存在前，必须先按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 查找现有 CLI；后续使用返回的完整路径。不得只凭 `command not found` 判定未安装或改走免 CLI 流程。

作者页和作品页共用这一套流程。页面本身由当前 Agent 使用正常的前端工具创建或修改；
ViceMe CLI 只负责确认目标、返回平台接口、校验 ZIP 结构、上传、预览、发布和回滚。
接口细节按需阅读 [runtime-api.md](references/runtime-api.md)。

面向用户的文字跟随当前语言。中文使用自然白话，只说“确认页面”“准备预览”“上传”
和“发布”等业务动作；不展示命令、权限名、账号 ID、错误码、原始 JSON 或内部状态，
也不告诉用户正在使用哪个内置 Skill。

## 两种调用模式

- **普通模式**：用户独立要求自定义作者页或作品页，执行下面的「固定流程」，必须先通过
  `$become-a-creator` 取得有效 Merchant，并使用在线 preview 确认后才公开发布。
- **创作者入驻模式**：只由 `$become-a-creator` 在普通申请提交后调用，输入必须带本次
  status 返回的准确 `creatorIdentity.profileUrl` 和暂停的 `merchant.id`。它只允许作者页，
  执行「入驻个人名片流程」；这个暂停租户不是经营资格，不得交给其他玩法。

## 入驻个人名片流程

1. 使用调用方给出的准确 `profileUrl` 和 Merchant，不让用户重选目标。先运行
   `viceme merchant page describe --target <profileUrl> --merchant <商家ID>`；CLI 会再次校验
   当前登录者、审核中的普通申请、DRAFT 作者身份和 handle 完全一致。
2. 调用方带有网页选择时直接执行，不得再次让用户选择风格：`mode=BONJOUR` 使用 Bonjour，
   `mode=IMPORT_EXISTING` 进入导入/自定义已有页面。只有兼容旧版调用且没有网页选择时，才让
   用户二选一：“Bonjour 风格模板”或“导入/自定义已有页面”。选择 Bonjour 后，必须把
   [bonjour-card](templates/bonjour-card/) 整个项目复制到用户工作目录作为实现起点，并先完整
   阅读其中 `README.md` 和 `DESIGN.md`；禁止从空白页面重新生成，禁止切换技术栈，也禁止
   把它当作可以自由发挥的灵感。该目录直接保留汪奕辰提供的 Profile Blocks 原型布局与交互，
   包括间距、分割线、标签、标题/副标题、添加 Block、作品导入、编辑按钮和图标；除非用户在
   看过本地预览后明确要求某项视觉改动，不得修改既有组件结构、class 名或 `src/styles.css`。
   导入 Vite/React 源码时先实际 build，上传 build 产物；不能因为根 `index.html` 存在就把开发
   入口当成发布产物。
   导入已有页面时先读 [导入与路径适配](references/import-routing.md)，在现有项目里完成路由和
   资源路径适配，不重做页面；仅有静态产物的包无需强行执行不存在的 build。
   模板已经通过现有 `context.read` 读取作者资料、作品摘要和作品封面，并通过
   `navigation.open` 打开站内作品；不得为它另建模板专用后端或上传流程。
3. 名称和头像使用 `qualification.user` 及平台 `context.read` 自动填写，简介有现值就复用，
   缺失才问用户。网页选择中的 `works` 和 `contacts` 是用户已经提供的资料，直接用于页面，
   不得重复询问；仍缺失时再按需收集。只收集两类可选内容：
   - 作品：标题、简介、链接、**一张作品卡片封面图**；这里的封面不是页面首屏大图。
   - 媒体：用户主动公开的联系方式，支持飞书链接、X / Twitter 链接、邮箱和 GitHub 主页链接。
   本期不支持视频；不添加文字、图片墙、App、公众号、高光、教育、工作经历、获奖等 Block。
4. 有源码时运行项目自己的 build；使用 [本地嵌套路径预览](references/import-routing.md#本地预览)
   打开这次准备上传的产物，而不是只看开发服务器根路径。由 AI 在内部检查首页、页面内跳转与
   刷新，然后把同一预览展示给用户；不增加用户设置项或额外确认。组件结构和
   `src/styles.css` 仍是模板唯一视觉依据，不看图重做。根据反馈修改，直到用户明确确认该本地版本。不得创建线上草稿库，也不得运行
   `viceme merchant page preview`。
5. 在 ZIP 根目录放置 `viceme-page.json`，运行 `viceme merchant page inspect --path <zip>`，
   再运行
   `viceme merchant page upload --path <zip> --target <profileUrl> --merchant <商家ID>`。
6. 上传返回 `VALIDATED` 后，运行
   `viceme merchant page status --target <profileUrl> --merchant <商家ID>`，使用上传得到的
   release ID 与真实当前 active release，立即运行
   `viceme merchant page publish <release-id> --expected-active <当前ID或none> --merchant <商家ID>`。
   本地预览已经获得确认，因此这里不再追问第二次发布确认。
7. 保存后打开原 `profileUrl`，实际检查 iframe 中的首页、页面内跳转和刷新；`PUBLISHED`、
   SDK 就绪或资源 HTTP 200 都不等于内容渲染成功。发现空白或应用自己的 NotFound 时按
   [导入与路径适配](references/import-routing.md) 处理，不宣布页面已经做好，不自动循环发布。
   验证通过后只返回调用方给出的原 `profileUrl`。申请审核期间同一路径只有登录着 ViceMe
   的本人可见；创作者资格获批后，同一 active release 自动公开，不需要个人名片复审或重传。

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
   已有页面先读 [导入与路径适配](references/import-routing.md)，保留页面功能与视觉，只做部署适配。
5. 在 ZIP 根目录放置 `viceme-page.json`，声明准确的页面 kind、真实 HTML 入口，以及
   页面实际需要的平台 capabilities。不要为了省事声明所有 capabilities。ZIP 可以采用
   任意目录结构，入口不要求叫 `dist/index.html`。
6. 运行 `viceme merchant page inspect --path <zip>`。只修复它返回的包结构问题，例如
   ZIP 无法读取、路径穿越、压缩炸弹、缺少 manifest、入口不存在或 manifest 不匹配；
   不借此增加浏览器源码限制。
7. 运行 `viceme merchant page preview --path <zip> --target <准确URL> --merchant <商家ID>`，
   打开命令返回的完整预览 URL，并把同一 URL 作为 Markdown 备用链接给用户。预览期间
   实际检查 iframe 内首页、页面内跳转与刷新；只有包内容发生修改时才重新运行预览命令，
   因为每次预览都会上传一个新的草稿 release。资源请求成功
   不代表应用路由正确，不得把空白或应用自己的 NotFound 当作可发布的预览。
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
