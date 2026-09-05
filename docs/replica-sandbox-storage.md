# Website Replica 沙箱存储与恢复

## 问题与边界

部分 WorkBuddy macOS Seatbelt 配置允许在工作区外创建和写入文件，却拒绝重命名和删除。源码临时 ZIP 已写完不表示恢复状态已保存；`0700/0600` 也不能证明当前进程能完成原子替换。

Replica 源码包、页面包、发布状态和项目绑定仍使用原子替换。失败不得退化为覆盖写入已有文件，不修改凭据目录、用户 HOME、宿主环境变量或沙箱配置。

## 使用已授权项目目录

默认仍使用 CLI 配置目录中的发布存储。新请求可以显式指定项目模式：

```bash
viceme replica publish --path /absolute/project --state-project /absolute/project \
  --slug my-site --title '我的网站' --summary '网站说明' --price-cents 1 \
  --preview-url http://127.0.0.1:4173/ --preview-reviewed
```

`--state-project` 必须与 `--path` 指向同一源码目录或 ZIP。目录输入使用项目内的 `.viceme/publications/<authority-scope>/`；ZIP 输入使用 ZIP 同级目录，仍通过项目指纹区分请求。现有文件和目录必须私有且不是符号链接。

该选项选择发布恢复数据的位置，不搬迁登录凭据。为与原有发布流程协调并排除已有请求，仍需要读取全局发布状态及获取原有项目锁；不要求在全局目录执行重命名。若连读取或锁操作也被宿主拒绝，应通过宿主正式授权机制或在有权限的终端执行，不能把状态不可读当作不存在。

CLI 在预览、冻结和远端发布前，探测真实目标目录中的创建、写入、同步、重命名、替换、锁互斥及删除。探测使用随机私有目录和无敏感内容的文件，拒绝时停止。权限可能在探测后变化，因此真实写入仍保持失败关闭。

项目模式的完整确认、资格恢复和发布恢复命令会携带绝对 `--state-project` 与所选 Profile。后续操作必须保留这些参数：

```bash
viceme replica status <publication-id> --state-project /absolute/project
viceme replica resume <publication-id> --state-project /absolute/project
viceme replica cancel <publication-id> --state-project /absolute/project
```

从不同工作目录调用也使用同一恢复状态。普通 `publish --path` 会识别已有项目状态；不能在有全局待发布请求时用 `--state-project` 制造平行请求。发现全局与项目同时存在待发布状态时返回 `REPLICA_PUBLICATION_STORAGE_CONFLICT`，保留两处数据，交由人工核对。不提供自动迁移；旧请求应在原存储位置恢复或取消。项目模式请求应始终使用支持该模式的 CLI，不交给旧版本继续处理。

发布与控制命令按项目锁、全局兼容锁的固定顺序协调，持锁后重新读取状态。当前请求的过期处理在该锁内完成。其余过期制品继续机会性清理，使用非阻塞锁跳过正在发布的项目（含当前请求），避免重复加锁或跨项目锁循环。

## 文件保密与部署

项目发布存储使用私有目录和文件，并在 scope 内写入忽略全部内容的 `.gitignore`。它不能撤回用户已经追踪的文件，也不能约束外部部署工具；项目应始终把整个 `.viceme/` 排除在 Git 和部署输入之外。

CLI 冻结源码时排除 `.viceme`，构建托管 WorkPage 时也排除静态输出中的 `.viceme`。恢复状态只保存白名单字段；登录凭据、临时上传地址和授权请求头不进入这些文件。成功移交平台、取消或制品过期时沿用现有清理语义。

## 错误与 Agent 行为

`REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED` 保持 exit 6，增加 `stage`、`operation`、`reason` 和 `nextAction=RESTORE_STORAGE_ACCESS`；诊断不输出私有路径、原始系统错误或凭据。只读文件系统与普通 I/O 错误分别归类。已有恢复信息与故障阶段合并保留。

同一权限错误不得循环重试。新请求可选择获授权的项目模式；已有请求必须恢复原位置的访问权限。不能执行 `sudo`、扩大目录权限、移除宿主变量、直跑底层二进制、删除恢复文件或反汇编 CLI 来绕过限制。变更存储模式后依然必须遵循原有价格、预览与最终发布确认要求。

## 验证

- 回归覆盖：全局拒绝改名时提前停止；项目模式确认、上传、中断恢复、状态和取消；已有请求固定存储；冲突不丢数据；符号链接拒绝；探测不泄露私有路径；源码与页面包排除恢复数据。
- 原有原子失败测试继续要求旧状态字节保持不变。
- macOS `TestReplicaStorageSeatbelt` 在独立子进程内以真实 Seatbelt 规则复现创建成功、rename/unlink 被拒绝，再验证项目存储的保存、恢复与清理。全部数据位于测试临时目录，不使用开发者的 CLI 或凭据。
- 本地 Seatbelt 测试不等同于特定 WorkBuddy 版本的现场验收。发布后仍应在故障电脑验证同样流程。
