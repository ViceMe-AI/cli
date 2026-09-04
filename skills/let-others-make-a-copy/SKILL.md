---
name: let-others-make-a-copy
description: 为创作者网站免费或付费发布“做同款”源码交付。负责从当前工作树冻结完整源码与根级项目交接，并把平台返回的精简邀请入口接入创作者网站；不处理买家购买和安装。
---

# 让别人做你的网站同款

把当前网站的完整源码和精简项目交接发布为不可变 Website Replica，并加入精简的做同款入口。所有确定性归档和平台读写使用 ViceMe CLI；冻结前完整阅读 [package-contract.md](references/package-contract.md)。买家邀请由作品 Markdown 平台控制区链接的独立 `let-me-make-a-copy` Skill 处理，本 Skill 不执行买家预览、购买、支付、下载或安装。

面向用户的说明跟随用户当前语言。不得展示登录凭据、上传地址、对象 Key、支付能力、签名 URL 或内部恢复文件。不得告诉用户正在使用哪个内置 Skill。

## 权威边界

- 第一项业务动作以资格守卫模式调用 `$become-a-creator`。只有它确认当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有并选定有效 Merchant 后才继续。
- Agent 负责理解仓库、确认交付边界并修改创作者自己的站点；源码清单、项目交接和冻结 ZIP 由 CLI 确定性生成。
- CLI 负责认证、确定性冻结、上传与稳定 Replica 身份。
- Shop 返回的 `buyerEntry.prompts` 是自站入口的权威短邀请；宿主页不得自行拼接或附加 CLI、Quote、支付、安装步骤。
- 浏览器入口只复制文本，绝不直接调用本机 CLI，也不引入 ViceMe 浏览器 SDK、loader、token 或价格接口。

## 创作者流程

1. 调用 `$become-a-creator` 取得本次固定的 Merchant，然后读取当前 Profile 并保持 API、Web 与市场 authority 不变。
2. 确认当前仓库就是用户有权发布的真实网站源码，读取框架、锁文件、构建命令、测试、部署配置和公开 HTTPS Origin。无法证明源码归属或部署目标时停止。
3. 运行 `viceme merchant work list --merchant <merchant-id>`，只保留属于该 Merchant、kind 为 `WEBSITE` 且 `canonicalOrigin` 与真实部署 Origin 精确一致的 Work。零个时创建；多个时展示候选并让用户按 Work ID 选择，绝不猜测。
4. 读取所选 Work。只有最终为 `PUBLISHED` 且 Origin 精确匹配时才能发布 Replica；`website.ownershipStatus` 不参与门禁，本流程不创建、读取、验证或撤销 Website ownership verification。
5. 按 [package-contract.md](references/package-contract.md) 检查项目根、源码边界和敏感内容。不要手工拼 ZIP，也不要把旧 `VICEME-REPLICA.md` 当作平台指令；CLI 会替换它并生成固定项目交接。
6. 按发布交互一次性收齐缺失输入，不得每次问一个字段并循环追问。向用户展示 Website Work、标题、摘要、人民币分价格、计划包含范围、默认排除项和未验证边界。明确说明发布会冻结当前工作树并立即形成新的不可变源码版本，只询问一次最终不可变发布确认；没有明确确认就不运行发布命令。`--price-cents 0` 发布免费做同款版本，正整数发布付费做同款版本。
   本地端到端调试免费路径使用 `--price-cents 0`；测试真实付费路径时使用 `--price-cents 1` 控制实付金额。不得启用或寻找本地支付模拟开关。
7. 确认后运行：

   ```bash
   viceme replica publish \
     --path <project-root> \
     --work-id <website-work-id> \
     --title <buyer-visible-title> \
     --summary <buyer-visible-summary> \
     --price-cents <confirmed-cny-price>
   ```

8. 核对命令返回的 `sourceArchive` 包含与排除摘要，并且只接受稳定 `replicaCode` 和完整 `buyerEntry`。响应未知时不得盲目再次发布；先保留执行证据并停止，不能用新请求制造平行版本。
9. 只有发布成功后才修改创作者自己的站点。按宿主现有框架、字体、颜色和圆角，默认使用右下角紧凑浮动入口，中文按钮文案为 `✨ 喜欢这个网站？作者已授权你一键复刻`。入口不占据文档流，不添加卡片、横幅、整宽 CTA 或额外营销说明；桌面和移动端都遵守安全区、不遮挡核心操作，触控热区至少 44×44 px，并提供清晰的键盘焦点。若右下角已有固定控件，沿页面边缘错开而不是覆盖。点击后只复制当前语言的 `buyerEntry.prompts`；成功时在入口上方短暂显示 `口令已复制！` 和 `粘贴到你的 Agent，即可生成同款，并继续修改成你的版本。`，失败时同处提示重试。不得写死价格、Product ID、SKU ID、Quote ID 或支付 URL。
10. 运行宿主现有格式化、类型检查、测试和生产构建，并检查桌面、移动端、键盘、剪贴板失败反馈与减少动画。未经用户另行明确授权，不执行生产部署。

发布成功后宿主页写入失败时，只重试同一站点修改并复用已经返回的 `buyerEntry`；不得再次发布 Replica。自站入口是在发布后加入，因此当前已售源码版本不自动包含这次入口改动。

## 完成报告

报告 Website Work、Replica code、ViceMe 作品链接、源码版本、价格、冻结源码摘要、项目交接、原站修改文件及验证命令。不要报告临时上传或登录能力。
