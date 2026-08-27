# Demo 环境 Agent 安装与 CLI 发布

本文适用于：

- Web/API：`https://demo.viceme.cn`
- CLI API Base URL：`https://demo.viceme.cn/api`
- CLI 安装包：`https://s3-demo.viceme.cn/viceme-shop/cli/releases`
- 源码分支：`codex/feat(repo)/unified-interaction-experience`
- 当前兼容线：`0.17.x`（源码 Release/Skill 版本为 `0.17.0`）
- 当前公开 Demo CLI：`0.17.101`

S3 凭证、Bucket 名和签名配置必须通过本地环境或 CI Secret 注入，不得写入仓库、命令历史、截图或聊天记录。

## 对外使用方式：让 Agent 完成安装

外部用户不需要手工判断操作系统、CPU 架构、安装路径或 Skill 目录。对外只提供一个受信任的 Agent 安装契约：

```text
https://s3-demo.viceme.cn/viceme-shop/agent-install.md
```

用户在 Codex、Claude Code 或 WorkBuddy 中发送：

> 请读取 https://s3-demo.viceme.cn/viceme-shop/agent-install.md，并严格按照其中的安装契约安装或更新 ViceMe Demo CLI。安装完成后绑定 demo Profile，但不要登录、发布或发起交易，除非我随后明确要求。

Agent 按契约完成：

```text
读取固定 HTTPS 契约
  → 选择 OS 与 CPU 安装包
  → 校验 SHA-256 并原子激活 CLI 与官方 Skills
  → 创建或选择 demo Profile
  → 校验 https://demo.viceme.cn/api
  → 执行 doctor 与 auth status
  → 返回安装结果
```

安装不会被解释为登录、上传、公开发布、购买或提交。需要登录时，由 Agent 在后续明确的业务请求中执行一次阻塞式 `viceme auth login`。

本文后续内容供平台发布与故障处理使用。普通外部用户优先使用上述 Agent 入口。

## 1. 多版本目录与版本规则

```text
https://s3-demo.viceme.cn/viceme-shop/cli/releases/
├── latest
├── install.sh
├── install.ps1
├── v0.17.100/
│   ├── viceme_0.17.100_darwin_amd64
│   ├── viceme_0.17.100_darwin_amd64.sha256
│   ├── viceme_0.17.100_darwin_arm64
│   ├── viceme_0.17.100_darwin_arm64.sha256
│   ├── viceme_0.17.100_linux_amd64
│   ├── viceme_0.17.100_linux_amd64.sha256
│   ├── viceme_0.17.100_linux_arm64
│   ├── viceme_0.17.100_linux_arm64.sha256
│   ├── viceme_0.17.100_windows_amd64.exe
│   ├── viceme_0.17.100_windows_amd64.exe.sha256
│   ├── viceme_0.17.100_windows_arm64.exe
│   └── viceme_0.17.100_windows_arm64.exe.sha256
└── v0.17.101/
    └── ...
```

规则：

1. 必须使用三段式稳定版本，如 `0.17.100`；当前安装器不接受 `0.17.100-demo.1`。
2. 每次构建使用从未发布过的新版本，禁止用不同二进制覆盖同一版本目录。
3. `latest` 只保存不带 `v` 的版本号，如 `0.17.101`。
4. 新版本必须大于待替换的已安装版本。同版本不同字节或降级安装会被激活保护拒绝。
5. 当前分支属于 `0.17.x` 兼容线，建议从预留 Demo 版本段（如 `0.17.100`）开始递增。

历史目录用于首次安装、指定版本安装和并行测试。同一 CLI 配置目录不支持直接降级；验证旧版本时使用独立安装目录和独立配置目录。

## 2. 从当前分支构建

发布机需要 Go 1.23+、AWS CLI 和 `sha256sum`：

```bash
git switch 'codex/feat(repo)/unified-interaction-experience'
git status --short --branch
make test
```

构建必须读取 Shop 当前分支 `apps/api/.env`，从 `COMMERCE_SKILL_SIGNING_SEED` 或其本地回退来源派生只含公钥的 `COMMERCE_SKILL_TRUST_KEYS`。不得把 Seed、Access Key 或 Secret Key 输出到终端或写入构建产物。

```bash
set -a
source /absolute/path/to/ViceMe-Shop/apps/api/.env
set +a

export DEMO_CLI_VERSION=0.17.101
export DEMO_CLI_COMMIT="$(git rev-parse --short=12 HEAD)"
export DEMO_RELEASE_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases

# 仅把派生后的 Ed25519 公钥传给构建；私密 Seed 仍只存在当前进程环境。
COMMERCE_SKILL_TRUST_KEYS="$(node --input-type=module <<'NODE'
import { createHash, createPrivateKey, createPublicKey } from 'node:crypto';

const seedText = process.env.COMMERCE_SKILL_SIGNING_SEED || createHash('sha256')
  .update(`viceme-commerce-skill-signing:${process.env.COMMERCE_TRUSTED_IDENTITY_SECRET}`)
  .digest('base64url');
const seed = Buffer.from(seedText, 'base64url');
if (seed.length !== 32 || !process.env.COMMERCE_SKILL_SIGNING_KEY_ID) process.exit(1);
const privateKey = createPrivateKey({
  key: Buffer.concat([Buffer.from('302e020100300506032b657004220420', 'hex'), seed]),
  format: 'der',
  type: 'pkcs8',
});
const publicKey = createPublicKey(privateKey)
  .export({ format: 'der', type: 'spki' })
  .toString('base64url');
process.stdout.write(`${process.env.COMMERCE_SKILL_SIGNING_KEY_ID}:${publicKey}`);
NODE
)"
export COMMERCE_SKILL_TRUST_KEYS
test -n "$COMMERCE_SKILL_TRUST_KEYS"

release_dir="$(mktemp -d)"
chmod 700 "$release_dir"

for target in \
  darwin/amd64 darwin/arm64 \
  linux/amd64 linux/arm64 \
  windows/amd64 windows/arm64
do
  goos="${target%/*}"
  goarch="${target#*/}"
  extension=""
  [ "$goos" = windows ] && extension=".exe"
  asset="viceme_${DEMO_CLI_VERSION}_${goos}_${goarch}${extension}"

  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-X github.com/ViceMe-AI/cli/internal/buildinfo.Version=${DEMO_CLI_VERSION} -X github.com/ViceMe-AI/cli/internal/buildinfo.Commit=${DEMO_CLI_COMMIT} -X github.com/ViceMe-AI/cli/internal/buildinfo.CommerceSkillTrustKeys=${COMMERCE_SKILL_TRUST_KEYS} -X github.com/ViceMe-AI/cli/internal/buildinfo.ReleaseBaseURL=${DEMO_RELEASE_BASE_URL}" \
    -o "$release_dir/$asset" \
    ./cmd/viceme

  (cd "$release_dir" && sha256sum "$asset" >"$asset.sha256")
done

cp installers/install.sh installers/install.ps1 "$release_dir/"
```

发布前至少验证发布机可执行的目标。例如 Apple Silicon macOS：

```bash
"$release_dir/viceme_${DEMO_CLI_VERSION}_darwin_arm64" version
(cd "$release_dir" && shasum -a 256 -c "viceme_${DEMO_CLI_VERSION}_darwin_arm64.sha256")
```

输出版本必须等于 `DEMO_CLI_VERSION`，commit 必须等于 `DEMO_CLI_COMMIT`。

## 3. 发布到 S3 Demo

下面使用 Shop 当前分支 `apps/api/.env` 中的 Access Key、Secret Key、Bucket 和 Region。该 `.env` 的 Endpoint 是本机 RustFS，因此发布时仅把 S3 API Endpoint 替换为 `https://s3-demo.viceme.cn`；其他 S3 配置仍以 `.env` 为准。Bucket `viceme-shop` 中的 `cli/releases/...` 通过 `https://s3-demo.viceme.cn/viceme-shop/cli/releases/...` 公开读取。

```bash
set -a
source /absolute/path/to/ViceMe-Shop/apps/api/.env
set +a

export DEMO_S3_ENDPOINT='https://s3-demo.viceme.cn'
export DEMO_S3_BUCKET="$S3_BUCKET"
export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
export AWS_DEFAULT_REGION="$S3_REGION"
export AWS_REQUEST_CHECKSUM_CALCULATION='WHEN_REQUIRED'
export AWS_RESPONSE_CHECKSUM_VALIDATION='WHEN_REQUIRED'

release_prefix="cli/releases/v${DEMO_CLI_VERSION}"

# 版本目录不可变。任一待发布对象已存在时都停止并换新版本号。
for file in "$release_dir"/viceme_*; do
  key="$release_prefix/${file##*/}"
  if aws --endpoint-url "$DEMO_S3_ENDPOINT" s3api head-object \
    --bucket "$DEMO_S3_BUCKET" --key "$key" >/dev/null 2>&1
  then
    echo "$key already exists; use a new version" >&2
    exit 1
  fi
done

for file in "$release_dir"/viceme_*; do
  aws --endpoint-url "$DEMO_S3_ENDPOINT" s3 cp "$file" \
    "s3://$DEMO_S3_BUCKET/$release_prefix/${file##*/}" \
    --cache-control 'public,max-age=31536000,immutable' \
    --only-show-errors
done

for file in install.sh install.ps1; do
  aws --endpoint-url "$DEMO_S3_ENDPOINT" s3 cp "$release_dir/$file" \
    "s3://$DEMO_S3_BUCKET/$release_prefix/$file" \
    --cache-control 'public,max-age=31536000,immutable' \
    --only-show-errors
  aws --endpoint-url "$DEMO_S3_ENDPOINT" s3 cp "$release_dir/$file" \
    "s3://$DEMO_S3_BUCKET/cli/releases/$file" \
    --cache-control 'public,max-age=300' \
    --only-show-errors
done

# 所有版本文件上传成功后，最后更新 latest。
latest_file="$(mktemp)"
printf '%s\n' "$DEMO_CLI_VERSION" >"$latest_file"
aws --endpoint-url "$DEMO_S3_ENDPOINT" s3 cp "$latest_file" \
  "s3://$DEMO_S3_BUCKET/cli/releases/latest" \
  --content-type 'text/plain' \
  --cache-control 'public,max-age=60' \
  --only-show-errors
rm -f "$latest_file"
```

不要删除旧的 `vX.Y.Z` 目录。发布后必须从公网重新下载校验：

```bash
public_base='https://s3-demo.viceme.cn/viceme-shop/cli/releases'
test "$(curl -fsSL "$public_base/latest" | tr -d '\r\n')" = "$DEMO_CLI_VERSION"

verify_dir="$(mktemp -d)"
asset="viceme_${DEMO_CLI_VERSION}_darwin_arm64"
curl -fsSL "$public_base/v${DEMO_CLI_VERSION}/$asset" -o "$verify_dir/$asset"
curl -fsSL "$public_base/v${DEMO_CLI_VERSION}/$asset.sha256" -o "$verify_dir/$asset.sha256"
(cd "$verify_dir" && shasum -a 256 -c "$asset.sha256")
rm -rf "$verify_dir"
```

## 4. 从未安装过 CLI

### macOS 或 Linux

安装 `latest`：

```bash
VICEME_REGION=cn \
VICEME_DOWNLOAD_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases \
sh -c "$(curl -fsSL https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.sh)"
```

安装指定版本：

```bash
VICEME_REGION=cn \
VICEME_VERSION=0.17.100 \
VICEME_DOWNLOAD_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases \
sh -c "$(curl -fsSL https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.sh)"
```

默认位置是 `$HOME/.local/bin/viceme`。若终端找不到命令：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Windows PowerShell

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$env:VICEME_REGION = "cn"
$env:VICEME_DOWNLOAD_BASE_URL = "https://s3-demo.viceme.cn/viceme-shop/cli/releases"
# 安装指定版本时取消下一行注释：
# $env:VICEME_VERSION = "0.17.100"
irm https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.ps1 | iex
```

默认位置是 `%LOCALAPPDATA%\ViceMe\bin\viceme.exe`。安装器会更新用户 PATH，首次安装后应新开 PowerShell 窗口。

### 绑定 Demo API

下载域名不决定业务 API。首次安装后必须显式创建 Demo Profile：

```bash
viceme profile add \
  --name demo \
  --api-base-url https://demo.viceme.cn/api \
  --use

viceme auth login
viceme auth status
viceme doctor
```

`auth status` 的 Profile 必须是 `demo`，API 必须是 `https://demo.viceme.cn/api`。凭证按 Profile 和 API Origin 隔离，正式环境凭证不能复用到 Demo。

## 5. 已有 CLI：替换为当前分支包

先记录现状：

```bash
command -v viceme
viceme version
viceme profile list
```

### 5.1 已有较旧的 standalone 版本

使用新版本和原安装目录重新运行安装器：

```bash
VICEME_REGION=cn \
VICEME_VERSION=0.17.100 \
VICEME_INSTALL_DIR="$HOME/.local/bin" \
VICEME_DOWNLOAD_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases \
sh -c "$(curl -fsSL https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.sh)"
```

安装器会校验 SHA-256，原子替换二进制，并同步该版本内置的官方 Skills。原有 Profile 和同 Origin 的登录凭证会保留。随后确认：

```bash
viceme version
viceme profile use demo
viceme auth status
viceme doctor
```

若没有 `demo` Profile，按上一节执行 `profile add`。

### 5.2 已有版本较新，或通过 npm/npx 安装

当前 Demo 兼容线是 `0.17.x`。如果现有 standalone 版本等于或高于目标版本，原地替换会返回 `BOOTSTRAP_DOWNGRADE_REFUSED`。如果现有版本来自 npm/npx，同一配置目录中切换为 standalone 会返回 `BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED`。两种情况都不要手工删除激活日志或 active-generation 文件。

推荐把 Demo CLI 安装到隔离目录，并让测试终端优先使用它：

```bash
export VICEME_CLI_CONFIG_DIR="$HOME/.viceme-cli-demo"
export VICEME_INSTALL_DIR="$HOME/.local/viceme-demo/bin"

VICEME_REGION=cn \
VICEME_VERSION=0.17.100 \
VICEME_DOWNLOAD_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases \
sh -c "$(curl -fsSL https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.sh)"

export PATH="$VICEME_INSTALL_DIR:$PATH"
viceme profile add --name demo --api-base-url https://demo.viceme.cn/api --use
viceme auth login
viceme doctor
```

使用 Demo CLI 的终端必须持续保留该 `VICEME_CLI_CONFIG_DIR` 和 PATH。这样不会破坏原 npm CLI 的激活状态、Profile 或凭证。

### 5.3 验证历史版本

同一配置目录禁止降级。为每个历史版本使用独立目录：

```bash
export VICEME_CLI_CONFIG_DIR="$HOME/.viceme-cli-demo-0.17.100"
export VICEME_INSTALL_DIR="$HOME/.local/viceme-demo/0.17.100/bin"

VICEME_REGION=cn \
VICEME_VERSION=0.17.100 \
VICEME_DOWNLOAD_BASE_URL=https://s3-demo.viceme.cn/viceme-shop/cli/releases \
sh -c "$(curl -fsSL https://s3-demo.viceme.cn/viceme-shop/cli/releases/install.sh)"

"$VICEME_INSTALL_DIR/viceme" version
```

## 6. 安装后的基本使用

用户可在安装了官方 Skills 的 Agent 中直接描述：

> 将这个 Skill 发布到 ViceMe Demo。

> 发布一个招聘服务，并在发布前让我确认场景分析、页面预览和最终交互。

终端核对：

```bash
viceme auth status
viceme merchant accounts
viceme doctor
```

可下载 Skill 包的入口：

```bash
viceme skill inspect --path ./my-skill
viceme skill publish --path ./my-skill
```

服务型 Work 必须经过场景分析、用户逐项确认、预览和最终激活，建议交给 `viceme-publish` Skill 编排，不要跳过确认门禁直接拼装底层 JSON。

## 7. 独立 Demo 更新通道

`VICEME_DOWNLOAD_BASE_URL` 控制首次安装器。本仓库的 Demo 构建还通过链接参数把 `ReleaseBaseURL` 固定为 `https://s3-demo.viceme.cn/viceme-shop/cli/releases`，因此安装后的 standalone CLI、自动 freshness gate 和 `viceme update` 都继续使用 Demo 通道，不会切换到正式 CN Release。

正式构建不注入该参数，仍按区域使用 `s3.viceme.cn` 或 `s3.viceme.ai`。不要把 Demo Release Base URL 注入正式包。

## 8. 常见问题

### 404

检查文件名和目录是否完全匹配：

```text
cli/releases/v<version>/viceme_<version>_<os>_<arch>[.exe]
cli/releases/v<version>/viceme_<version>_<os>_<arch>[.exe].sha256
```

### checksum mismatch

二进制与 `.sha256` 不是同一次构建，或版本目录被覆盖。停止发布，换新版本号重新构建；不要修改已公开目录。

### BOOTSTRAP_DOWNGRADE_REFUSED

目标版本低于现有版本，或同版本对应了不同二进制。换用更高的新版本，或用独立安装和配置目录验证历史版本。

### 登录后仍访问正式环境

```bash
viceme profile list
viceme profile use demo
viceme auth status
```

确认 API Base URL 为 `https://demo.viceme.cn/api`，然后重新执行 `viceme auth login`。

### Agent 没有发现新 Skills

执行 `viceme doctor`，然后新建 Codex、Claude Code 或 WorkBuddy 会话。Agent 通常只在会话启动时发现 Skills。
