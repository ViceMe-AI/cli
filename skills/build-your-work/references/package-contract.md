# 包结构与安全合同

## 根结构

最终 ZIP 不得套 `<skill-name>/` 外层目录，根级直接包含 `SKILL.md`：

```text
package.zip
├── SKILL.md
├── START.md
├── agents/openai.yaml
├── site/                 # 网站转换必需
├── assets/               # 仅在有独立复用价值时
├── template/             # 仅在需要可编辑起点时
└── references/           # 仅在详细说明有价值时
```

现有 Skill 保留真实所需结构，不强行添加空 `site/`、`assets/`、`template/` 或 `references/`。

## 平台红线

- ZIP 压缩后不超过 10 MiB；
- 解压后所有文件合计不超过 10 MiB；
- 单文件不超过 10 MiB；
- 文件数不超过 1000；
- 单条 UTF-8 路径不超过 512 字节；
- 无绝对路径、路径穿越、反斜杠路径或重复路径；
- 无符号链接、设备文件和其他特殊文件；
- 无加密条目或异常压缩比条目；
- 根级存在唯一 `SKILL.md`，其 `name` 合规；
- ZIP 条目按路径排序，时间戳、权限和压缩方式确定化。

至少排除：

```text
.git/ .hg/ .svn/ node_modules/ .next/ .turbo/
.cache/ __pycache__/ coverage/ .venv/ venv/
.env* .npmrc .pypirc .DS_Store Thumbs.db
id_rsa id_ed25519
```

输入始终排除 `viceme-dist/`，防止把历史包或 staging 递归打入 ZIP。Site 可以保留真正运行所需的最终构建产物，但不得保留构建依赖和开发缓存。

## 凭据检查

扫描私钥标记、AWS access key、GitHub token、`sk-...` 形式 token 和常见凭据赋值。明显的示例占位符可以保留。发现疑似真实密钥时：

1. 停止交付，不创建或保留一个被称为成功的 ZIP；
2. 只报告文件相对路径和匹配类型，不显示密钥值；
3. 提醒创作者撤销或轮换凭据；
4. 不自行修改创作者源文件。

## Reference Site

网站包的 `site/` 是视觉、交互、动效、资产和验收参考。部署产物根映射为 `site/` 根，例如 `dist/index.html` 与 `dist/assets/app.js` 应成为 `site/index.html` 与 `site/assets/app.js`，不保留额外 `dist/` 包装层，也不扁平化、重命名或重新分组内部文件。

如果缺少某个文件或运行依赖会让名称、品牌、核心内容、CORE 交互、主要层级、关键动画或主要体验路径消失，不能把降级包当作成功。停止并报告阻塞，或者取得创作者对具体改变的明确选择后把它作为独立创作修改重新验收。

## 可选发布伴随信息

封面、图库候选、标题、简介、使用说明、建议价格和试用次数可以在 ZIP 完成后另行准备，但不得因此阻塞 `package.zip`，不得未经确认把建议价格当最终价格，也不得把非运行所需的发布素材放入买家包。平台没有定义导入合同前，不声称伴随 JSON 可以直接导入。
