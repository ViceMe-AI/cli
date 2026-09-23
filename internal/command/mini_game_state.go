package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/gofrs/flock"
)

const miniGameStatePath = ".viceme/mini-game.v1.json"
const miniGameLockPath = ".viceme/mini-game.lock"
const miniGameMaxFileBytes = 8 << 20

type miniGameState struct {
	SchemaVersion  int               `json:"schemaVersion"`
	EndpointOrigin string            `json:"endpointOrigin"`
	ProjectPath    string            `json:"projectPath"`
	Selection      miniGameSelection `json:"selection"`
	Files          map[string]string `json:"files"`
	// 未完成的生成仅允许磁盘保留旧或目标哈希，重跑即可安全向前恢复。
	Pending map[string]string `json:"pending,omitempty"`
}

func miniGameProject(project string) (string, error) {
	absolute, err := filepath.Abs(project)
	if err != nil {
		return "", miniGameStorageError(err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", output.Validation("MINI_GAME_PROJECT_INVALID", "项目必须是已存在的真实目录，不能是符号链接").WithHint("指定小游戏源码所在的真实目录后重跑 --project。")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", miniGameStorageError(err)
	}
	return canonical, nil
}

func lockMiniGameProject(project string, write bool) (*flock.Flock, error) {
	if err := miniGameSafePath(project, miniGameLockPath); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(project, filepath.FromSlash(miniGameLockPath))
	if !write {
		if _, err := os.Lstat(lockPath); errors.Is(err, os.ErrNotExist) {
			return nil, nil
		} else if err != nil {
			return nil, miniGameStorageError(err)
		}
		lock := flock.New(lockPath)
		locked, err := lock.TryRLock()
		if err != nil {
			return nil, miniGameStorageError(err)
		}
		if !locked {
			return nil, miniGameBusy()
		}
		return lock, nil
	}
	if _, err := privatepath.EnsureDirectory(filepath.Join(project, ".viceme")); err != nil {
		return nil, miniGameStorageError(err)
	}
	if _, err := privatepath.EnsureFile(lockPath); err != nil {
		return nil, miniGameStorageError(err)
	}
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, miniGameStorageError(err)
	}
	if !locked {
		return nil, miniGameBusy()
	}
	return lock, nil
}

func miniGameBusy() error {
	return output.Policy("MINI_GAME_BUSY", "同一项目已有接入或检查正在进行").WithHint("等待当前命令结束后重跑，不要删除锁文件。")
}

func miniGameSafePath(project, relative string) error {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return output.Validation("MINI_GAME_PATH_UNSAFE", "小游戏路径必须留在项目目录内").WithHint("改用项目内的相对脚本路径，不要使用符号链接。")
	}
	current := project
	parts := strings.Split(clean, string(os.PathSeparator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return miniGameStorageError(err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !info.IsDir()) || (index == len(parts)-1 && !info.IsDir() && !info.Mode().IsRegular()) {
			return output.Validation("MINI_GAME_PATH_UNSAFE", "拒绝读取或覆盖符号链接及特殊文件").WithHint("把该路径改为项目内的真实普通文件或目录，保留原文件后再重跑。").WithDetails(map[string]string{"file": relative})
		}
	}
	return nil
}

func readMiniGameFile(project, relative string) ([]byte, bool, error) {
	if err := miniGameSafePath(project, relative); err != nil {
		return nil, false, err
	}
	path := filepath.Join(project, filepath.FromSlash(relative))
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, miniGameStorageError(err)
	}
	if !before.Mode().IsRegular() || before.Size() > miniGameMaxFileBytes {
		return nil, false, output.Validation("MINI_GAME_FILE_INVALID", "小游戏脚本和状态必须是大小不超过 8 MiB 的普通文件").WithDetails(map[string]string{"file": relative})
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, miniGameStorageError(err)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, false, miniGameBusy()
	}
	data, err := io.ReadAll(io.LimitReader(file, miniGameMaxFileBytes+1))
	if err != nil {
		return nil, false, miniGameStorageError(err)
	}
	if len(data) > miniGameMaxFileBytes {
		return nil, false, output.Validation("MINI_GAME_FILE_INVALID", "小游戏文件超过 8 MiB 上限").WithDetails(map[string]string{"file": relative})
	}
	if err := miniGameSafePath(project, relative); err != nil {
		return nil, false, err
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(current, before) {
		return nil, false, miniGameBusy()
	}
	return data, true, nil
}

func loadMiniGameState(project string) (*miniGameState, error) {
	data, exists, err := readMiniGameFile(project, miniGameStatePath)
	if err != nil || !exists {
		return nil, err
	}
	if err := privatepath.RequirePrivateFile(filepath.Join(project, filepath.FromSlash(miniGameStatePath))); err != nil {
		return nil, miniGameStorageError(err)
	}
	var state miniGameState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil || decoder.Decode(&struct{}{}) != io.EOF || state.SchemaVersion != 1 || state.ProjectPath == "" || state.EndpointOrigin == "" || !validMiniGameFileHashes(state.Files, false) || (state.Pending != nil && !validMiniGameFileHashes(state.Pending, true)) || (state.Pending == nil && len(state.Files) != 2) {
		return nil, output.Validation("MINI_GAME_STATE_INVALID", "小游戏受管状态损坏或包含未知数据").WithHint("从版本控制或备份恢复 .viceme/mini-game.v1.json 及配套两份受管文件；不要手工猜写哈希。")
	}
	if _, err := resolveMiniGameSelection(miniGameSelection{}, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func validMiniGameFileHashes(files map[string]string, complete bool) bool {
	if files == nil || (complete && len(files) != 2) || len(files) > 2 {
		return false
	}
	for name, hash := range files {
		if name != miniGameRuntimePath && name != miniGameConfigPath {
			return false
		}
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash {
			return false
		}
	}
	return true
}

func validateMiniGameManagedFiles(project string, state *miniGameState) error {
	for _, name := range []string{miniGameRuntimePath, miniGameConfigPath} {
		data, exists, err := readMiniGameFile(project, name)
		if err != nil {
			return err
		}
		if state == nil {
			if exists {
				return miniGameModified(name, "文件已存在但没有 ViceMe 管理状态")
			}
			continue
		}
		committed := state.Files[name]
		if !exists {
			if committed != "" {
				return miniGameModified(name, "已安装的受管文件被删除")
			}
			continue
		}
		hash := miniGameDigest(data)
		if hash != committed && (state.Pending == nil || hash != state.Pending[name]) {
			return miniGameModified(name, "受管文件被手动修改")
		}
	}
	return nil
}

func miniGameModified(name, reason string) error {
	return output.Validation("MINI_GAME_MANAGED_FILE_MODIFIED", reason+"，已停止且不会覆盖").WithHint("先把自定义逻辑移到宿主 JS；从备份恢复该受管文件，再运行 integrate。若无备份，将两份受管文件和 .viceme/mini-game.v1.json 一起移到项目外保留后重新接入，不要仅删除状态。").WithDetails(map[string]string{"file": name})
}

func saveMiniGameState(project string, state *miniGameState) error {
	if err := miniGameSafePath(project, miniGameStatePath); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return miniGameStorageError(err)
	}
	if err := privatefile.WriteAtomic(filepath.Join(project, filepath.FromSlash(miniGameStatePath)), data, ".mini-game-state-*.tmp"); err != nil {
		return miniGameStorageError(err)
	}
	return nil
}

func installMiniGameFiles(project string, state *miniGameState, files map[string][]byte) ([]string, error) {
	updated := []string{}
	planned := map[string]string{}
	for _, name := range []string{miniGameRuntimePath, miniGameConfigPath} {
		planned[name] = miniGameDigest(files[name])
		current, exists, err := readMiniGameFile(project, name)
		if err != nil {
			return nil, err
		}
		if !exists || miniGameDigest(current) != planned[name] {
			updated = append(updated, name)
		}
	}
	if len(updated) == 0 && state.Pending == nil {
		return updated, nil
	}
	if err := miniGameSafePath(project, miniGameRuntimePath); err != nil {
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(project, "viceme"), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, miniGameStorageError(err)
	}
	// 先保存实际现有生成，再保存新目标，使中断恢复不接受任意人工改动。
	for _, name := range []string{miniGameRuntimePath, miniGameConfigPath} {
		data, exists, err := readMiniGameFile(project, name)
		if err != nil {
			return nil, err
		}
		if exists {
			hash := miniGameDigest(data)
			if hash != state.Files[name] && (state.Pending == nil || hash != state.Pending[name]) {
				return nil, miniGameModified(name, "受管文件在接入期间发生变化")
			}
			state.Files[name] = hash
		}
	}
	state.Pending = planned
	if err := saveMiniGameState(project, state); err != nil {
		return nil, err
	}
	for _, name := range updated {
		if err := validateMiniGameManagedFiles(project, state); err != nil {
			return nil, err
		}
		if err := miniGameSafePath(project, name); err != nil {
			return nil, err
		}
		if err := privatefile.WriteAtomic(filepath.Join(project, filepath.FromSlash(name)), files[name], ".mini-game-asset-*.tmp"); err != nil {
			return nil, miniGameStorageError(err)
		}
	}
	state.Files, state.Pending = planned, nil
	if err := saveMiniGameState(project, state); err != nil {
		return nil, err
	}
	return updated, nil
}

func miniGameDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func miniGameStorageError(err error) error {
	return output.Internal("MINI_GAME_STORAGE_FAILED", "无法安全读写小游戏受管文件", err).WithHint("检查项目文件权限；若前次接入中断，保留 .viceme 状态并重跑 integrate；不要删除锁或覆盖不相关文件。")
}
