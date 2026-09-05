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
)

const TrialDisabledMarker = "<!-- viceme-trial-disabled:v1"

// SuspendTrialSkills replaces only a matching marketplace trial's entrypoint.
// The caller holds the shared Go/Python Product lock. Native destination locks
// also fence official installs and incomplete transactions in other profiles.
// No credential, install manifest, script, reference, or user output is removed.
func SuspendTrialSkills(environment Environment, productID, purchaseURL, installDocURL string) (int, error) {
	known, err := resolveKnownTargets("trial-scan", environment)
	if err != nil {
		return 0, err
	}
	roots := map[string]bool{}
	for _, target := range known {
		roots[filepath.Dir(target.path)] = true
	}
	var destinations []string
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
	for _, destination := range destinations {
		changed, err := suspendTrialEntry(destination, productID, purchaseURL, installDocURL)
		if changed {
			count++
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func suspendTrialEntry(directory, productID, purchaseURL, installDocURL string) (bool, error) {
	// Cheap ownership check first: unrelated and unmanaged Skills need no lock
	// or write probe. Recheck the same identity after taking the path lock.
	if !trialProductOwnsDirectory(directory, productID) {
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
	if !trialProductOwnsDirectory(directory, productID) {
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
	replacement, match := suspendedTrialMarkdown(original, filepath.Base(directory), productID, purchaseURL, installDocURL)
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
	if err != nil || !bytes.Equal(current, original) || !trialProductOwnsDirectory(directory, productID) {
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

func trialProductOwnsDirectory(directory, productID string) bool {
	for _, filename := range []string{directory, filepath.Join(directory, ".viceme"), filepath.Join(directory, installManifestPath)} {
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
	return json.Unmarshal(raw, &manifest) == nil && manifest.ProductID == productID && manifest.ReleaseID != ""
}

func suspendedTrialMarkdown(original []byte, skillName, productID, purchaseURL, installDocURL string) ([]byte, bool) {
	meta, err := parseFrontmatter(original)
	if err != nil || meta.Name != skillName {
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
	escape := strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D")
	notice := fmt.Sprintf("%s\n\n# 试用已结束\n\n本技能的免费试用次数已用完，当前已停用。不得继续执行原技能任务，也不得调用目录中保留的脚本或参考资料来继续试用。\n\n请打开[购买页面](<%s>)完成购买。然后按[官方安装说明](<%s>)安装或更新 ViceMe CLI，登录购买时使用的账号，运行 `viceme skill install %s --owned` 安装正式版。\n\n正式版安装成功后，重新读取 SKILL.md 再继续任务。重装试用版不会恢复试用次数。\n",
		disabled, escape.Replace(purchaseURL), escape.Replace(installDocURL), productID)
	if bytes.HasPrefix(original, []byte("---\r\n")) {
		notice = strings.ReplaceAll(notice, "\n", "\r\n")
	}
	return append(bytes.Clone(original[:prefixEnd]), []byte(notice)...), true
}
