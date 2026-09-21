# 公共支付功能

`host-presentation.md` 是支付展示与等待边界的唯一维护源，属于 CLI 公共支付能力，不属于任何 Skill。它管理订单入口、聊天二维码、内置浏览器、付款状态与恢复边界；各 Skill 继续拥有购买授权、商品身份和付款后的业务操作。

Go 的 `PaymentPresentationGuide()` 与 `paymentPresentationHint()` 使用同源规则；Python Skill 运行包携带 `guides/host-presentation.md`。`use-a-skill` 与 `let-me-make-a-copy` 的 `references/host-presentation.md` 是自动生成的分发副本，不手工维护。独立复制脚本也嵌入同一份指引。

修改源文件后运行 `make release-manifest`。生成器同时更新引用副本、Python 运行包与独立脚本，`make release-manifest-check` 校验它们与唯一源一致。新增购买 Skill 通过生成器的消费清单加入，不复制一套支付流程。
