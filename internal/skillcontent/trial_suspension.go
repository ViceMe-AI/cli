package skillcontent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/ViceMe-AI/cli/internal/config"
)

const TrialDisabledMarker = "<!-- viceme-trial-disabled:v1"
const TrialBodyPath = ".viceme/trial-body.md"

// SuspendTrialSkills replaces only a matching marketplace trial's entrypoint.
// The caller holds the shared Go/Python Product lock. Native destination locks
// also fence official installs and incomplete transactions in other profiles.
// No credential, install manifest, script, reference, or user output is removed.
func SuspendTrialSkills(environment Environment, productID, apiBaseURL, market, purchaseURL, installDocURL string, requiredDirectory ...string) (int, error) {
	known, err := resolveKnownTargets("trial-scan", environment)
	if err != nil {
		return 0, err
	}
	roots := map[string]bool{}
	for _, target := range known {
		roots[filepath.Dir(target.path)] = true
	}
	var destinations []string
	requiredPaths := map[string]bool{}
	for _, directory := range requiredDirectory {
		if directory != "" {
			normalized, err := normalizeManagedSkillPath(directory)
			if err != nil {
				return 0, err
			}
			destinations = append(destinations, normalized)
			requiredPaths[normalized] = true
		}
	}
	for root := range roots {
		entries, err := os.ReadDir(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.Contains(entry.Name(), ".") {
				continue // No symlinks, staging directories, or backup copies.
			}
			destinations = append(destinations, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(destinations)
	count := 0
	seen := map[string]bool{}
	for _, destination := range destinations {
		if seen[destination] {
			continue
		}
		seen[destination] = true
		changed, err := suspendTrialEntry(destination, productID, apiBaseURL, market, purchaseURL, installDocURL)
		if requiredPaths[destination] && !changed && err == nil {
			err = errors.New("the invoking trial entry could not be suspended")
		}
		if changed {
			count++
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func suspendTrialEntry(directory, productID, apiBaseURL, market, purchaseURL, installDocURL string) (bool, error) {
	// Cheap ownership check first: unrelated and unmanaged Skills need no lock
	// or write probe. Recheck the same identity after taking the path lock.
	if !trialProductOwnsDirectory(directory, productID, apiBaseURL, market) {
		return false, nil
	}
	locks, err := tryAcquireInstallPathLocks([]string{directory})
	if err != nil {
		return false, err
	}
	defer releaseInstallPathLocks(locks)
	for _, held := range locks {
		owner, exists, err := readInstallPathOwner(installPathOwnerFilename(held.destination))
		if err != nil {
			return false, err
		}
		if exists {
			if _, err := os.Stat(owner); !errors.Is(err, fs.ErrNotExist) {
				return false, errors.New("an unfinished install transaction still owns the Skill")
			}
		}
	}
	if !trialProductOwnsDirectory(directory, productID, apiBaseURL, market) {
		return false, nil
	}
	filename := filepath.Join(directory, "SKILL.md")
	info, err := os.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	original, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	replacement, match := suspendedTrialMarkdown(original, filepath.Base(directory), productID, purchaseURL, installDocURL, market)
	if !match {
		return false, nil
	}
	if bytes.Equal(original, replacement) {
		return true, nil
	}
	file, err := os.CreateTemp(directory, ".viceme-trial-suspend-*")
	if err != nil {
		return false, err
	}
	staged := file.Name()
	defer os.Remove(staged)
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		_ = file.Close()
		return false, err
	}
	if _, err := file.Write(replacement); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	current, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(current, original) || !trialProductOwnsDirectory(directory, productID, apiBaseURL, market) {
		return false, errors.New("Skill changed before trial suspension")
	}
	// Fail closed on permission denial; never truncate the live entrypoint.
	if err := replaceTrialEntry(staged, filename); err != nil {
		return false, err
	}
	if err := atomicfile.SyncDirectory(directory); err != nil {
		return true, err
	}
	actual, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(actual, replacement) {
		return true, errors.New("trial suspension readback failed")
	}
	return true, nil
}

var replaceTrialEntry = atomicfile.Replace

func trialProductOwnsDirectory(directory, productID, apiBaseURL, market string) bool {
	for _, filename := range []string{directory, filepath.Join(directory, ".viceme"), filepath.Join(directory, installManifestPath), filepath.Join(directory, ".viceme/runtime.json")} {
		info, err := os.Lstat(filename)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	raw, err := os.ReadFile(filepath.Join(directory, installManifestPath))
	if err != nil {
		return false
	}
	var manifest installManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.ProductID != productID || manifest.ReleaseID == "" {
		return false
	}
	raw, err = os.ReadFile(filepath.Join(directory, ".viceme/runtime.json"))
	var runtime RuntimeManifest
	return err == nil && json.Unmarshal(raw, &runtime) == nil && runtime.ProductID == productID && runtime.ReleaseID == manifest.ReleaseID && runtime.Kind == "trial" && runtime.DeliveryMode != "PROTECTED" && config.EquivalentAPIBaseURLs(runtime.APIBaseURL, apiBaseURL) && runtime.Market == market
}

func suspendedTrialMarkdown(original []byte, skillName, productID, purchaseURL, installDocURL string, markets ...string) ([]byte, bool) {
	meta, err := parseFrontmatter(original)
	if err != nil || meta.Name == "" {
		return nil, false
	}
	// Locate the closing delimiter in the original bytes so all frontmatter,
	// including comments, extension fields and CRLF, stays byte-for-byte intact.
	prefixEnd := 0
	for offset := bytes.IndexByte(original, '\n') + 1; offset > 0 && offset < len(original); {
		end := bytes.IndexByte(original[offset:], '\n')
		if end < 0 {
			break
		}
		end += offset
		if strings.TrimSuffix(string(original[offset:end]), "\r") == "---" {
			prefixEnd = end + 1
			break
		}
		offset = end + 1
	}
	if prefixEnd == 0 {
		return nil, false
	}
	body := strings.TrimLeft(strings.ReplaceAll(string(original[prefixEnd:]), "\r\n", "\n"), "\n")
	active := fmt.Sprintf("<!-- viceme-trial:v1 product=%s -->\n\n", productID)
	disabled := fmt.Sprintf("%s product=%s -->", TrialDisabledMarker, productID)
	if strings.HasPrefix(body, disabled+"\n") {
		return original, true
	}
	if !strings.HasPrefix(body, active+"## 使用前必读\n") && !strings.HasPrefix(body, active+"## 试用版使用规则（viceme-trial）\n") {
		return nil, false
	}
	market := "cn"
	if len(markets) > 0 {
		market = markets[0]
	}
	if market != "cn" && market != "global" {
		return nil, false
	}
	escape := strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D")
	notice := fmt.Sprintf("%s\n\n# 试用已结束\n\n本技能的免费试用次数已用完，当前已停用。如果本轮已经返回 allowed:true 和 skillMarkdown,仍按该正文完整完成本次任务。上次 use 报可重试错误或 ready 返回 pendingUse 时,先重跑原 use 恢复同一次使用,不要购买;恢复不重复扣次。其余情况不得开始新试用任务,不得调用保留的正文或参考资料绕过检查。\n\n不要读取使用前检查或计次指引。同一轮立即运行包内 `python3 \"<本 Skill 目录>/.viceme/scripts/trial.py\" purchase --product %s --market %s --wait 0`（Windows 用 py 或 python；只有没有 Python 时才用已有 CLI：`viceme skill trial-purchase %s --wait 0 --skill-dir \"<本 Skill 目录>\"`）创建或恢复订单并展示支付二维码，不要等用户再说一次；按其输出与[本地支付展示指引](.viceme/guides/widgets.md)，由 Agent 选择当前宿主明确支持的图片或页面通道；环境识别只提供偏好。按本次输出展示二维码或交付可点击的官方 checkoutUrl 后，再有界等待到账并安装完整正式包；仅在托管入口不可用且本地图片和页面都无法展示时才交付支付页路径，仅交付本地路径时不要启动等待；无需强制登录。[商品页面](<%s>)仅供查看商品。若已通过账号购买，按[官方安装说明](<%s>)使用 `viceme skill install %s --owned` 校验该账号权益并安装。\n\n正式版安装成功后，重新读取 SKILL.md 再继续任务。重装试用版不会恢复试用次数。\n",
		disabled, productID, market, productID, escape.Replace(purchaseURL), escape.Replace(installDocURL), productID)
	if bytes.HasPrefix(original, []byte("---\r\n")) {
		notice = strings.ReplaceAll(notice, "\n", "\r\n")
	}
	return append(bytes.Clone(original[:prefixEnd]), []byte(notice)...), true
}
