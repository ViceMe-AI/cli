# CLI + Shop dev 测试交付

## 运行入口与边界

手动工作流为 .github/workflows/dev-test-delivery.yml。先让工作流定义进入默认 main 分支，再从 main 的 Actions 页面运行，输入当前 dev 完整 SHA。工作流只 checkout dev 并核对 SHA；不会打标签、发布 npm、创建生产 Release 或写生产桶。不要改用现有 release.yml 测试功能。

每次运行以 dev-RUN_ID-RUN_ATTEMPT-SHA12 标识；可执行文件仍为开发版本 dev，源码兼容版本规则不变。独立构建目录、commit、摘要共同标识产物。字面 dev 不作为发布目录或共享不可变身份。手动 dev 更新通过完整卸载旧独立安装后再安装新包，生产版本比较与激活逻辑保持不变。

## 一次性配置

GitHub 创建 dev Environment，通过部署分支策略仅允许 main 工作流使用。当前只有国内 dev 环境；使用仅能访问国内 dev 桶的凭据，禁止复制有生产桶或业务桶写权限的 credentials。工作流不会创建桶或修改权限。

| 类型 | 名称 | 要求 |
| --- | --- | --- |
| Secret | VICEME_DEV_CN_S3_ENDPOINT、VICEME_DEV_CN_S3_ACCESS_KEY_ID、VICEME_DEV_CN_S3_SECRET_ACCESS_KEY | CN dev 桶 |
| Secret | VICEME_DEV_CN_S3_HTTPS_PROXY | CN 上传需要代理时配置，不能泄漏凭据 |
| Variable | VICEME_DEV_COMMERCE_SKILL_TRUST_KEYS | dev Commerce 签名验证公钥；不可使用私钥 |
| Variable | VICEME_DEV_TEMPLATE_CATALOG_TRUST_KEYS | 模板目录签名验证公钥；当前只读公共模板仍使用原发布源 |

公开访问为 https://s3.dev.viceme.cn/dev/，endpoint 为 https://s3.dev.viceme.cn（不含桶路径）。不发布海外 dev，也不要求海外 dev 密钥。配置只读对象访问，禁止公开列桶和写入。发布工具固定 bucket=dev 且校验对应公开 origin，不能传 start/skills 桶。

Shop 的 Web、Admin、API 配置 DEPLOYMENT_ENV=dev，并配置 dev 公共 Web/API 地址。NODE_ENV 继续使用 production 构建，MARKET_REGION 只区分 CN/GLOBAL。部署设置与环境备份遵守 Shop 仓库规范。

## 与生产 CI 的对应关系

复用生产的 Go/Node 版本、npm ci、release-manifest 重新生成与干净检查、make check、npm-package-check、信任公钥校验、六平台 Go 编译，以及同一个 S3 客户端的上传和公开回读校验。dev 另加安装/恢复及 S3 发布 race 测试。

两条工作流并非逐步骤相同：dev 手动冻结 dev SHA，生成独立 ZIP 与 dev 安装工具，只发布国内 dev 桶；生产生成稳定标签、签名安装契约、GitHub Release 和 npm 包，并发布两个生产区域。生产更新和签名安装器流程保持原样。

## 产物和发布顺序

构建执行现有质量门禁，生成六平台 ZIP，每包含 CLI、独立 viceme-dev-setup 工具、BUILD.json、SHA256SUMS 和说明。源码必须干净且 HEAD 与输入相同。仅 --allow-dirty 本地自测包可标注 sourceDirty=true，S3 发布器拒绝该产物。

构建在临时源码快照中渲染 dev 官方 Skills/脚本/指引，再重建 runtime 和 release manifest；不修改 Go 的生产版本或更新逻辑。官方 Skills、Python runtime 和安装指引中的后续下载固定到该构建目录，避免安装过程跨构建混用。SDK 和签名模板属于独立发布的只读资源，保留原版本化分发源；本工作流不重发 SDK/模板。Commerce 安装契约仅新增精确的 dev 指引地址配对，仍拒绝混用区域或生产/dev 地址。

1. 写入不可变 builds/BUILD_ID/ 下的所有平台包、官方 Skills、指引和 runtime；已存在而字节不同即失败。
2. 下载公开不可变产物并核对字节与缓存头。
3. 按 run/attempt 单调更新 dev 当前 skills/、start/ 入口，最后更新 delivery.json。旧运行不得覆盖新运行。
4. 回读当前入口。任一发布或回读失败则该 run 失败，不宣称完成。重跑使用新 attempt，不复用原构建身份。

Agent 从同一次 run 下载 artifact，匹配 delivery.json/SHA256SUMS 和公开下载；交付文档使用不可变 URL。工作流/权限/桶未就绪时报告 Actions 地址及输入，由维护者手动处理，不用旧包兜底。

## 安装与恢复

测试包中的 viceme-dev-setup 是开发交付工具，不安装进生产 CLI，也不作为 viceme 子命令。

- install：核验平台、commit、摘要，显式 --replace-standalone 后退役旧独立安装，调用既有 bootstrap 激活，创建/使用 dev Profile。
- uninstall：要求安装包记录的 --sha256 与明确 --destination；仅卸载 dev，保留配置、凭据、官方 Skills 注册表和用户作品。
- 退役工具持有 activation/member/install/automatic-update 锁，拒绝任何未恢复的正式安装日志，校验记录与二进制归属。先保存专用退役记录和可执行文件备份，再清除与已移除程序匹配的 generation。中断以同一请求重入；不会清除较新的 generation。
- 生产恢复：dev 工具卸载成功后使用官方生产安装器，然后明确选择生产 Profile。仅切 Profile 或仅删除程序均不等价于恢复生产。
- npm 仍由 npm 管理，工具不会把 npm shim 覆盖成独立二进制。未知或不一致的安装状态必须保留并诊断。
- 不清空整个 ~/.viceme-cli、宿主 Skills 目录、系统凭据管理器或已购买 Skill 的许可证。清凭据使用原 CLI 的 profile/auth 命令，先明确范围。

本地验证使用隔离 HOME/config/bin；交叉编译不证明所有平台已运行，Linux 容器不证明 macOS/Windows 凭据服务或 GUI 宿主成功。业务验收从 dev 网页原始口令进行。
