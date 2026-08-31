# 通用商家 Product 流程

服务、实物/定制商品和普通 ViceMe 官方服务都走此流程。可售 Product 前必须先有一个真实创作者 Work：

1. 一个具有永久创作者/slug 身份和版本化公开内容的 `SERVICE` Work；
2. 该 Work 下的商家 Product 和不可变销售规格；
3. 平台生成并签名、且只绑定该 Product 的购买 Skill。

生成的 Skill 是进入 ViceMe 报价、支付、同一会话订单状态和履约的入口，不是创作者的服务实现。

## 0. 赞赏发布预检

原始请求同时包含赞赏，且接入流程尚未完成精确 SDK 发布预检时，先加载 `viceme-tip`。预检必须早于任何 Work 或 Product 创建、更新、编译、发布、SDK access 或宿主页写入；失败就停止，不留下新的 Work 或 Product。若 `viceme-tip` 已完成预检并因缺少合格 Work 回到这里，不得重复预检。Product 激活并原子发布真实 Merchant Work 后，再返回 `viceme-tip` 对该 Work 完成接入。

## 1. 确定获准商家

使用 `$viceme-creator-onboarding` 已确认并由用户选定的商家，然后运行：

```text
viceme merchant product templates --merchant <merchant-account-id>
```

只有当前用户作为唯一 `MerchantAccountMember(role=OWNER)` 拥有的有效 MerchantAccount 才能创作 Product。资格不存在或仍在审核时停止并交回 `$viceme-creator-onboarding`；不得在本流程创建申请或平行商家。公开 Product 还要求 Merchant 关联一个有效 CreatorAccount，其稳定 handle 决定永久 Work 路由。CreatorAccount 和已验证外部身份只用于署名，不提供写权限。

确认返回的有效创作模板为 `GENERIC_MERCHANT`。当前商家 Product 只能使用 `MANUAL_PROCESSING` 和 `SHIPMENT`；不得声称存在 provider adapter。ViceMe 官方手机充值也使用相同流程和 `MANUAL_PROCESSING`。官方创作者是普通 Creator/Merchant/OWNER 关系，不是特殊平台主体。

## 2. 收集完整销售定义

使用自然语言，只询问尚未明确的业务事实，并取得：

- Work 标题、永久 slug、公开简介/正文、标签、使用或服务说明，以及这是新 Work 还是已有 Work 的新 revision；
- Product 标题、slug、市场、适用时的双语简介/说明、描述、标签、类别名称和 `PUBLIC`、`UNLISTED` 或 `PRIVATE` 可见性；
- 每个选项、SKU、选项组合、币种、以分计价的单价、库存规则和数量范围；
- 定价规则 `FIXED_SKU`、`UNIT_QUANTITY` 或 `UNIT_PLUS_SHIPPING`，以及适用的默认/地区运费；
- 每个用户填写字段的稳定 key、显示名称、类型、是否必填、敏感级别、允许访问者、保留规则和语义角色；
- 履约顺序：人工处理、发货或两者；
- 服务模式：照片打印、人工充值等单订单工作使用 `FULFILLMENT_ONLY`；只有招聘等需要长期分阶段案件时使用 `LONG_RUNNING`；
- `LONG_RUNNING` 必须提供有序公开阶段名称和至少一个终态；案件查询仍只限最初 Commerce Session。

不得收集凭证、密码、验证码、token 或由用户填写的金额。价格始终来自服务端 SKU。发货必须要求收件人、联系电话和地址。用户已提供的内容不重复询问。

## 3. 创建或更新真实 Service Work

所有公开 Product 都需要真实 Work。已有 Work 从 `viceme merchant work list --merchant <merchant-account-id>` 解析并更新新的草稿 revision。新服务写入私有严格 JSON，再运行：

```text
viceme merchant work create --input <work.json>
```

照片打印的单次履约 Work 示例：

```json
{
  "kind": "SERVICE",
  "merchantAccountId": "merchant-uuid",
  "clientRequestId": "uuid",
  "market": "CN",
  "slug": "photo-printing-service",
  "title": "照片打印服务",
  "content": {
    "summary": "上传照片并选择规格，由商家打印后发货。",
    "bodyMarkdown": "## 服务说明\n\n按所选规格打印照片并通过快递发货。",
    "templateType": "service",
    "tags": ["照片", "打印"],
    "usageInstructions": "选择规格、上传照片并填写收件信息。",
    "serviceInstructions": "付款后由商家打印并发货。",
    "media": [],
    "actionConfig": {}
  },
  "service": {
    "intakeSchema": {},
    "stages": [
      { "code": "PROCESSING", "label": "处理中", "terminal": false },
      { "code": "COMPLETED", "label": "已完成", "terminal": true }
    ],
    "policy": {
      "caseMode": "FULFILLMENT_ONLY",
      "currentSessionOnly": true
    }
  }
}
```

长期招聘服务可使用 `ACCEPTED`、`SOURCING`、`CANDIDATES_RECOMMENDED`、`INTERVIEWING` 和终态 `COMPLETED`，并把 `policy.caseMode` 设为 `LONG_RUNNING`。不得把公司、联系人、电话、地址、照片或其他用户数据放入 Work 内容或 `intakeSchema`；这些属于 Product 用户合同和加密订单合同。

使用返回的 Work `id` 作为必填 `subjectWorkId`，再写完整 Product JSON 并运行：

```text
viceme merchant product create --input <product.json>
```

外层对象：

```json
{
  "merchantAccountId": "merchant-uuid",
  "clientRequestId": "uuid",
  "subjectWorkId": "work-uuid",
  "authoringTemplateCode": "GENERIC_MERCHANT",
  "authoringInput": {}
}
```

命令后删除私有临时请求文件。返回 ID 和 revision 只保留在当前任务状态，不得把用户数据或访问 token 写入源码或 Skill 目录。

## 4. 编译并预览准确候选内容

运行：

```text
viceme merchant product compile <product-id> \
  --merchant <merchant-account-id> \
  --expected-revision <revision>
```

编译必须没有验证错误，并返回一个 `candidateSalesSpecVersionId`、一个 `candidateDigest` 和一个 `candidatePurchaseSkill`。随后为 Product 使用的 Work revision 创建私有预览：

```text
viceme merchant work preview create <work-id> \
  --merchant <merchant-account-id> \
  --expected-revision <revision>
```

展示返回的 `htmlUrl` 和 `markdownUrl`。二者必须投影同一 Work revision、同一已编译 Product、价格、用户合同、服务阶段和购买 Skill 候选。启用前预览不得显示可用安装或购买动作，也不得把预览 URL 当作永久公开身份。不再使用的预览用 `viceme merchant work preview revoke <work-id> <preview-id> --merchant <merchant-account-id>` 撤销。

向用户展示：

- Product 标题、可见性、所有 SKU 和准确币种/价格；
- 数量与运费规则；
- 全部用户填写字段，并标明必填和敏感项；
- 有序履约步骤；
- 生成购买 Skill 的 `stableName`、Release ID、manifest digest 和分发预期；
- 当前候选内容准确的 HTML 与 Markdown 预览 URL；
- 支付成功与履约成功是两个不同状态。

不得启用过期候选。Work 或 Product 任一修改后都要重新 compile，并重新生成 HTML/Markdown 预览。

## 5. 一次确认并原子启用

只问一次：用户是否确认已展示定义并立即开放该 Product。明确说明启用后可产生真实订单和支付。修改要求不算确认。

明确确认后，完全根据同一次 compile 响应构造 activation JSON：

```json
{
  "merchantAccountId": "merchant-uuid",
  "expectedRevision": 2,
  "candidateSalesSpecVersionId": "uuid",
  "candidateDigest": "64-lowercase-hex",
  "candidateSkillReleaseId": "uuid",
  "candidatePurchaseManifestDigest": "64-lowercase-hex"
}
```

运行：

```text
viceme merchant product activate <product-id> --input <activation.json>
```

返回 `productDetailUrl`、`purchaseSkillStableName` 和 Product ID。启用事务会同时发布当前 Work revision、Product 销售规格、适用的 ServiceOffering 版本和签名购买 Skill。不得在 Product 启用前单独发布 Work，以免公开一个无法执行动作的页面。

详情 URL 必须使用平台响应中永久且无 locale 前缀的 `/{creatorHandle}/{workSlug}` 路由；不得自行生成 `/share`、带 locale 的路径或 `/products/{purchase-skill-stable-name}`。告诉商家页面上的“复制到 WorkBuddy / Codex”会生成平台拥有的 `commerce-skill-install.md` 合同和准确的签名 Skill 安装命令。不得暴露临时 ZIP URL、digest、过期时间或 API 来源，也不得仅为完成发布而启动用户 Commerce Session。

商家明确要求启用后做本地用户侧冒烟测试时，只使用已安装购买 Skill 的 `viceme commerce flow start --skill <stable-name>`。把返回 `localContextId` 保留在该测试对话中，不得创建替代 context 查询已有订单。

生成的稳定名称必须以 `viceme-` 开头。`DIRECT=PUBLISHED` 表示签名包可直接安装；`WORKBUDDY=READY` 表示可以人工提交 WorkBuddy 审核，不代表已经在 WorkBuddy 商店公开。

## 照片打印参考结构

以下缩略 `authoringInput` 表示 10 元打印费加 5 元默认运费；实际使用用户确认的名称、规格、价格和规则：

```json
{
  "title": "照片打印",
  "slug": "photo-printing",
  "market": "CN",
  "summaryZhCn": "上传照片，商家打印后发货",
  "description": "按所选规格打印照片并通过快递发货。",
  "usageInstructionsZhCn": "选择规格、上传照片并填写收件信息。",
  "tags": ["照片", "打印"],
  "visibility": "PUBLIC",
  "merchantTypeName": "照片打印服务",
  "options": [
    {
      "code": "size",
      "name": "尺寸",
      "values": [{ "code": "six-inch", "label": "6 英寸" }]
    }
  ],
  "skus": [
    {
      "code": "six-inch",
      "title": "6 英寸照片",
      "currency": "CNY",
      "priceCents": 1000,
      "inventoryPolicy": "MADE_TO_ORDER",
      "selectedOptions": { "size": "six-inch" },
      "attributes": {}
    }
  ],
  "quantity": { "min": 1, "max": 100 },
  "buyerContract": {
    "fields": [
      {
        "key": "photos",
        "label": "待打印照片",
        "kind": "image",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "minItems": 1,
        "maxItems": 20,
        "maxSizeBytes": 10485760,
        "contentTypes": ["image/jpeg", "image/png"]
      },
      {
        "key": "recipient_name",
        "label": "收件人",
        "kind": "text",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "RECIPIENT_NAME",
        "maxLength": 100,
        "multiline": false
      },
      {
        "key": "contact_phone",
        "label": "联系电话",
        "kind": "phone",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "CONTACT_PHONE",
        "allowedCountryCodes": ["+86"]
      },
      {
        "key": "shipping_address",
        "label": "收货地址",
        "kind": "address",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "SHIPPING_ADDRESS",
        "allowedRegions": ["CN"]
      }
    ]
  },
  "fulfillment": [
    { "capabilityCode": "MANUAL_PROCESSING", "configuration": {} },
    { "capabilityCode": "SHIPMENT", "configuration": {} }
  ],
  "pricingPolicyCode": "UNIT_PLUS_SHIPPING",
  "shippingPricing": {
    "defaultAmountCents": 500,
    "regionAmounts": {}
  }
}
```

## 恢复和生命周期

- create/compile/activate 返回结果不确定时，列出同一商家的 Works 或 Products，并按 client request ID、Work ID 或 Product ID 恢复。不得为了摆脱不确定性创建替代项。
- `viceme merchant product suspend` 停止新销售，但保留已有 session/order 恢复；`archive` 是终态，在仍有可支付订单或可恢复 session 时可能被拒绝。
- 不支持跨 session 查询订单。用户必须继续使用该购买 Skill 最初创建的准确本地 Commerce Session。
