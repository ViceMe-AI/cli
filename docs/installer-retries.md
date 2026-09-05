# 安装权限重试与 Agent 命令定位

官方独立安装器将下载校验与 CLI 激活分开。CLI 仍然是安装事务、权限预检和中断恢复的唯一所有者；下载缓存不是恢复日志。

macOS / Linux 使用用户专属临时缓存 `viceme-bootstrap-cache-<uid>`，拒绝符号链接和其他用户拥有的目录，并将权限限制为 `0700`。Windows 使用用户临时目录中的 `viceme-bootstrap-cache`，继承用户临时目录的 ACL 并拒绝目录重解析点。缓存键包含完整发布来源、版本、平台资产名和 SHA-256；每次运行仍从选定来源取得校验值，并在独立临时副本上重新校验。

只有已验证下载遇到 CLI 策略退出码 6 时才保留缓存。用户通过宿主正式审批获得权限后，重跑同一个安装命令即可复用下载。成功或其他激活失败会删除该缓存项；超过两天的遗留下载在后续安装时清理。并发进程各自验证和执行临时副本，保留缓存使用原子替换，不持有额外的激活锁。文件损坏时重新取得官方资产并校验，校验失败不得激活。

首次安装的权限错误提示重跑同版本安装器；已安装 CLI 的显式更新继续使用 `viceme update`。下载缓存不可替代或删除任何激活日志、Profile、凭据或已安装 Skill。

## Agent 后续调用

安装成功返回的 `data.destination` 是当前对话后续调用的入口。CLI 安装器不能修改已运行 Agent 的父进程环境，交互终端的 shell 启动文件也不代表 Agent 沙箱的 PATH。

每个新对话在需要 CLI 时，先使用官方 `creator-tools/scripts/resolve-cli.sh` 或 `resolve-cli.ps1` 定位入口。优先保留 PATH 中现有的命令（包括 npm launcher）；否则查找 `VICEME_INSTALL_DIR` 或官方默认目录。定位器只输出完整路径，不安装、不更新、不推断安装方法、不读取凭据。调用方验证 `version` 后，在所有独立工具调用中沿用带引号的完整路径。自定义目录没有加入 PATH 时，调用方须保留 `VICEME_INSTALL_DIR` 或安装返回的 destination。

定位器退出 127 表示支持的位置未找到 CLI；已有文件的执行权限或 CLI 恢复错误不等于未安装。禁止因此切换安装方法或选择免 CLI 分支，也不需要全盘搜索、读取 shell 配置或重启宿主。

## 验证

`go test ./installers` 用独立进程执行安装脚本，覆盖权限失败后的下载计数、缓存破坏、来源和版本隔离、并发重试、成功及其他失败清理。Windows 对应测试由 Windows Go 测试运行器执行。

`go test ./internal/skillcontent` 验证嵌入并安装的定位脚本在干净 PATH、默认和自定义路径、包含空格的路径以及已有 launcher 场景下的行为。`make check`、`go test -race ./internal/command ./internal/update` 和 `make npm-package-check` 验证命令、安装恢复、发布清单及 npm 消费者。

真实 WorkBuddy 的审批弹窗和 Agent 是否遵循指引，需要在新版本发布后用其正式宿主回归；进程级测试不声称消除了宿主内部的沙箱试跑。
