---
name: let-others-make-a-copy
description: 为创作者网站开通“做同款”源码交付。复用 $become-a-creator 完成资格守卫，生成完整源码 ZIP 与根级部署文档，通过 ViceMe 发布稳定口令，并在创作者自己的站点加入平台控制的复制提示词入口。
---

# 让别人做你的网站同款

把当前网站的完整源码和可执行部署说明发布为不可变 Website Replica，再在原网站加入“通过 ViceMe 做这个网站同款”的提示词入口。所有确定性平台读写使用 ViceMe CLI；打包前完整阅读 [package-contract.md](references/package-contract.md)。

面向用户的说明跟随用户当前语言。不得展示登录凭据、上传地址、对象 Key、支付能力、签名 URL 或内部恢复文件。不得告诉用户正在使用哪个内置 Skill。

## 权威边界

- 第一项业务动作以资格守卫模式调用 `$become-a-creator`。只有它确认当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有并选定有效 Merchant 后才继续；本 Skill 不自行登录、申请或选择 Merchant。
- Agent 负责理解仓库、生成部署文档、准备源码包并修改创作者自己的站点。
- CLI 负责认证、上传、稳定 Replica 身份、Quote、订单、下载许可和安全原子安装。
- Shop 返回的 `buyerEntry.prompts` 是自站入口的权威提示词；宿主页不得自行拼接购买流程。
- 浏览器入口只复制文本，绝不直接调用本机 CLI，也不引入 ViceMe 浏览器 SDK、loader、token 或价格接口。

## 创作者流程

1. 调用 `$become-a-creator` 取得本次固定的 Merchant，然后读取当前 Profile 并保持 API、Web 与市场 authority 不变。
2. 确认当前仓库就是用户有权发布的真实网站源码，读取框架、锁文件、构建命令、测试、部署配置和公开 HTTPS Origin。无法证明源码归属或部署目标时停止。
3. 运行 `viceme merchant work list --merchant <merchant-id>`，只保留属于该 Merchant、kind 为 `WEBSITE` 且 `canonicalOrigin` 与真实部署 Origin 精确一致的 Work。零个时创建；多个时展示候选并让用户按 Work ID 选择，绝不猜测。
4. 读取所选 Work。只有最终为 `PUBLISHED` 且 Origin 精确匹配时才能发布 Replica；`website.ownershipStatus` 不参与门禁，本流程不创建、读取、验证或撤销 Website ownership verification。
5. 按 [package-contract.md](references/package-contract.md) 在项目根生成 `VICEME-REPLICA.md`，并准备根目录就是项目根的完整源码 ZIP。部署文档和 ZIP 都不能含 secret。
6. 向用户展示 Website Work、标题、摘要、人民币分价格、部署文档、归档范围和排除项。明确说明发布会立即形成新的不可变收费源码版本，只询问一次；没有明确确认就不运行发布命令。
7. 确认后运行：

   ```bash
   viceme replica publish \
     --path <source.zip> \
     --work-id <website-work-id> \
     --title <buyer-visible-title> \
     --summary <buyer-visible-summary> \
     --price-cents <confirmed-cny-price>
   ```

8. 只接受命令返回的稳定 `replicaCode` 和完整 `buyerEntry`。响应未知时不得盲目再次发布；先保留本地归档和执行证据并停止，不能用新请求制造平行版本。
9. 只有发布成功后才修改创作者自己的站点。按宿主现有框架、组件、字体、颜色和间距加入可键盘操作的入口，显示自然文案“通过 ViceMe 做这个网站同款”，复制当前页面语言对应的 `buyerEntry.prompts`。不得写死价格、Product ID、SKU ID、Quote ID 或支付 URL。
10. 运行宿主现有格式化、类型检查、测试和生产构建，并检查桌面、移动端、键盘、剪贴板失败反馈与减少动画。未经用户另行明确授权，不执行生产部署。

发布成功后宿主页写入失败时，只重试同一站点修改并复用已经返回的 `buyerEntry`；不得再次发布 Replica。自站入口是在发布后加入，因此当前已售源码版本不自动包含这次入口改动。

## 买家提示词合同

入口提示词必须让买家 Agent 执行以下顺序：

1. 询问一个尚不存在的新目标目录，运行 `viceme replica install <replica-code> --target <directory>`。
2. CLI 只创建并返回真实 Quote；Agent 展示商品、币种、总价和有效期，然后停止，不创建订单。
3. 只有买家明确确认该 Quote 后，原样重跑命令并追加 `--confirm`。Quote 变化或过期必须重新展示、重新确认。
4. 支付并安全安装后，读取新目录根级 `VICEME-REPLICA.md`，按其中步骤部署并继续买家原任务，不得停在“下载成功”。
5. 始终不得覆盖已有目录。

## 完成报告

报告 Website Work、Replica code、ViceMe 作品链接、源码版本、价格、部署文档、原站修改文件及验证命令。不要报告临时上传、支付、下载或登录能力。
