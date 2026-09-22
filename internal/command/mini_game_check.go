package command

import (
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"golang.org/x/net/html"
)

// 只读取 HTML 入口及其本地脚本/静态 import 图，不执行宿主代码。
func checkMiniGameReferences(project string, manifest api.MiniGameIntegration) ([]miniGameIssue, error) {
	issues := []miniGameIssue{}
	purchases := map[string]bool{}
	references := map[string]bool{}
	known := map[string]bool{}
	for _, item := range manifest.Items {
		known[item.Alias] = true
	}
	entries := []string{}
	count := 0
	err := filepath.WalkDir(project, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return miniGameStorageError(walkErr)
		}
		if filename == project {
			return nil
		}
		relative, err := filepath.Rel(project, filename)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".viceme", "node_modules", "vendor", "viceme", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		count++
		if count > 10000 {
			return output.Validation("MINI_GAME_PROJECT_TOO_LARGE", "项目超过 10000 个待扫描文件").WithHint("将 --project 指向实际小游戏入口目录，不要指向包含多个项目或依赖缓存的父目录。")
		}
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".html" || ext == ".htm" {
			entries = append(entries, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		issues = append(issues, miniGameIssue{Code: "HTML_ENTRY_MISSING", Fix: "指定包含小游戏 HTML 入口的 --project；在入口按顺序引用 viceme/mini-game-commerce.js、viceme/mini-game-config.js，再加载原游戏脚本。"})
	}
	seenIssues := map[string]bool{}
	appendIssue := func(issue miniGameIssue) {
		key := fmt.Sprintf("%s:%s:%d:%s", issue.Code, issue.File, issue.Line, issue.Alias)
		if !seenIssues[key] {
			issues = append(issues, issue)
			seenIssues[key] = true
		}
	}
	for _, entry := range entries {
		content, _, err := readMiniGameFile(project, entry)
		if err != nil {
			return nil, err
		}
		tokenizer := html.NewTokenizer(strings.NewReader(string(content)))
		runtimeCount, configCount, runtimeIndex, configIndex, scriptIndex := 0, 0, -1, -1, 0
		firstCall := -1
		visited := map[string]bool{}
		var scan func(string, string, int) error
		scan = func(filename, source string, index int) error {
			if visited[filename] {
				return nil
			}
			visited[filename] = true
			tokens := miniGameJSTokens(source)
			for pos := range tokens {
				token := tokens[pos]
				if token.kind != 'i' {
					continue
				}
				if token.text == "ViceMeMiniGame" && pos+3 < len(tokens) && tokens[pos+1].text == "." && tokens[pos+2].kind == 'i' && tokens[pos+3].text == "(" {
					if pos > 0 && tokens[pos-1].text == "." && (pos < 2 || tokens[pos-2].kind != 'i' || (tokens[pos-2].text != "window" && tokens[pos-2].text != "globalThis")) {
						continue
					}
					if firstCall < 0 {
						firstCall = index
					}
					method := tokens[pos+2].text
					if method != "createPurchaseCard" && method != "isUnlocked" && method != "redeemLicense" && method != "importLicenseImage" {
						continue
					}
					if pos+5 >= len(tokens) || tokens[pos+4].kind != 's' || (tokens[pos+5].text != ")" && tokens[pos+5].text != ",") {
						appendIssue(miniGameIssue{Code: "DYNAMIC_ALIAS", File: filename, Line: token.line, Fix: "静态检查不推断动态别名。将此付费点改为 ViceMeMiniGame." + method + "(\"实际道具别名\")，或在分支里分别使用明确的字面量别名；不要新增伪调用应付检查。"})
						continue
					}
					alias := tokens[pos+4].text
					if !known[alias] {
						appendIssue(miniGameIssue{Code: "UNKNOWN_ALIAS", File: filename, Alias: alias, Line: token.line, Fix: "将此调用的字面量别名改为本次 items 中的真实 alias；显示名称变化不改变 alias，不要自行生成新 ID。"})
						continue
					}
					if method == "createPurchaseCard" {
						purchases[alias] = true
					} else {
						references[alias] = true
					}
				}
				if token.text != "import" && token.text != "export" {
					continue
				}
				// 仅跟进静态 import 'x' / import ... from 'x' / export ... from 'x'。
				module := ""
				if token.text == "import" && pos+1 < len(tokens) && tokens[pos+1].kind == 's' {
					module = tokens[pos+1].text
				} else {
					for next := pos + 1; next < len(tokens) && next < pos+80; next++ {
						if tokens[next].text == ";" || tokens[next].text == "(" {
							break
						}
						if tokens[next].text == "from" && next+1 < len(tokens) && tokens[next+1].kind == 's' {
							module = tokens[next+1].text
							break
						}
					}
				}
				if module == "" || !strings.HasPrefix(module, ".") {
					continue
				}
				target, local := miniGameScriptPath(filename, module)
				if !local {
					appendIssue(miniGameIssue{Code: "SCRIPT_PATH_INVALID", File: filename, Fix: "静态 import 必须使用留在项目内的本地相对路径。"})
					continue
				}
				if miniGameIgnoredScript(target) {
					continue
				}
				if len(visited) > 1000 {
					return output.Validation("MINI_GAME_SCRIPT_LIMIT", "单个小游戏入口的静态脚本图超过 1000 个文件").WithHint("指定不含依赖构建树的实际小游戏入口。")
				}
				data, exists, err := readMiniGameFile(project, target)
				if err != nil {
					return err
				}
				if !exists {
					appendIssue(miniGameIssue{Code: "SCRIPT_MISSING", File: target, Fix: "恢复该静态 import 指向的本地 JavaScript 文件，或修正原宿主 import 路径。"})
					continue
				}
				if err := scan(target, string(data), index); err != nil {
					return err
				}
			}
			return nil
		}
		for {
			typeOf := tokenizer.Next()
			if typeOf == html.ErrorToken {
				if tokenizer.Err() != io.EOF {
					return nil, miniGameStorageError(tokenizer.Err())
				}
				break
			}
			if typeOf != html.StartTagToken {
				continue
			}
			token := tokenizer.Token()
			if token.Data == "base" {
				appendIssue(miniGameIssue{Code: "HTML_BASE_UNSUPPORTED", File: entry, Fix: "移除或改造影响本地脚本解析的 <base>，使用项目内明确相对路径；检查器不会猜测远程 base。"})
			}
			if token.Data != "script" {
				continue
			}
			attrs := map[string]string{}
			for _, attr := range token.Attr {
				attrs[attr.Key] = attr.Val
			}
			typ := strings.ToLower(attrs["type"])
			if typ != "" && typ != "module" && typ != "text/javascript" && typ != "application/javascript" {
				continue
			}
			scriptIndex++
			if src := attrs["src"]; src != "" {
				target, local := miniGameScriptPath(entry, src)
				if !local {
					continue
				}
				if target == miniGameRuntimePath || target == miniGameConfigPath {
					if target == miniGameRuntimePath {
						runtimeCount++
						runtimeIndex = scriptIndex
					} else {
						configCount++
						configIndex = scriptIndex
					}
					_, async := attrs["async"]
					_, deferScript := attrs["defer"]
					_, noModule := attrs["nomodule"]
					if async || deferScript || typ == "module" || noModule {
						appendIssue(miniGameIssue{Code: "MANAGED_SCRIPT_LOADING", File: entry, Fix: "两份 ViceMe 脚本使用普通顺序 <script src=\"...\"></script>；移除 async、defer、type=module 与 nomodule，放在游戏入口脚本之前。"})
					}
					continue
				}
				if miniGameIgnoredScript(target) {
					continue
				}
				data, exists, err := readMiniGameFile(project, target)
				if err != nil {
					return nil, err
				}
				if !exists {
					appendIssue(miniGameIssue{Code: "SCRIPT_MISSING", File: target, Fix: "恢复 HTML 引用的本地脚本，或修正 src 相对路径。"})
					continue
				}
				if err := scan(target, string(data), scriptIndex); err != nil {
					return nil, err
				}
			} else {
				if tokenizer.Next() == html.TextToken {
					if err := scan(fmt.Sprintf("%s#script-%d", entry, scriptIndex), string(tokenizer.Text()), scriptIndex); err != nil {
						return nil, err
					}
				}
			}
		}
		if runtimeCount != 1 || configCount != 1 || runtimeIndex >= configIndex || (firstCall >= 0 && configIndex >= firstCall) {
			appendIssue(miniGameIssue{Code: "HTML_SCRIPT_REFERENCES", File: entry, Fix: "在此 HTML 中各引用一次运行库和配置（按入口位置调整相对路径）：<script src=\"viceme/mini-game-commerce.js\"></script><script src=\"viceme/mini-game-config.js\"></script>；顺序不可颠倒，放在调用 ViceMeMiniGame 的游戏脚本之前，删除重复引用。"})
		}
	}
	for _, item := range manifest.Items {
		if item.Status == "ACTIVE" && !purchases[item.Alias] {
			issues = append(issues, miniGameIssue{Code: "ACTIVE_ALIAS_MISSING", Alias: item.Alias, Fix: "在 HTML 实际加载的游戏 JS 中，为此道具的真实购买按钮调用 await ViceMeMiniGame.createPurchaseCard(" + strconv.Quote(item.Alias) + ")，并展示返回的付款卡；不要在注释、未加载文件或受管配置中伪造引用。"})
		} else if item.Status != "ACTIVE" && !references[item.Alias] {
			issues = append(issues, miniGameIssue{Code: "ITEM_ALIAS_MISSING", Alias: item.Alias, Fix: "在 HTML 实际加载的游戏 JS 中保留此道具的权益引用，例如 await ViceMeMiniGame.isUnlocked(" + strconv.Quote(item.Alias) + ")；下架或归档只移除新购买入口，不得删除已有许可证的兑换与权益恢复。"})
		}
	}
	return issues, nil
}

func miniGameScriptPath(owner, source string) (string, bool) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || strings.Contains(parsed.Path, "\\") {
		return "", false
	}
	var target string
	if strings.HasPrefix(parsed.Path, "/") {
		target = path.Clean(strings.TrimPrefix(parsed.Path, "/"))
	} else {
		target = path.Clean(path.Join(path.Dir(strings.Split(owner, "#")[0]), parsed.Path))
	}
	if target == ".." || strings.HasPrefix(target, "../") || target == "." {
		return "", false
	}
	return target, true
}

func miniGameIgnoredScript(filename string) bool {
	for _, part := range strings.Split(filename, "/") {
		if part == "vendor" || part == "node_modules" || part == ".viceme" {
			return true
		}
	}
	return filename == miniGameRuntimePath || filename == miniGameConfigPath
}

type miniGameJSToken struct {
	text string
	kind byte
	line int
}

// 有界词法检查，不把注释、字符串、模板文本或正则正文当成调用。
// 模板插值也不作为可验证的接入点；真实付费调用应放在普通 JavaScript 语句中。
func miniGameJSTokens(source string) []miniGameJSToken {
	tokens := []miniGameJSToken{}
	line := 1
	templateBraces := []int{}
	for index := 0; index < len(source); {
		ch := source[index]
		if ch == '\n' {
			line++
			index++
			continue
		}
		if ch == ' ' || ch == '\r' || ch == '\t' {
			index++
			continue
		}
		if ch == '/' && index+1 < len(source) && source[index+1] == '/' {
			for index < len(source) && source[index] != '\n' {
				index++
			}
			continue
		}
		if ch == '/' && index+1 < len(source) && source[index+1] == '*' {
			index += 2
			for index < len(source) {
				if source[index] == '\n' {
					line++
				}
				if source[index] == '*' && index+1 < len(source) && source[index+1] == '/' {
					index += 2
					break
				}
				index++
			}
			continue
		}
		start, startLine := index, line
		if ch == '`' || (ch == '}' && len(templateBraces) > 0 && templateBraces[len(templateBraces)-1] == 1) {
			if ch == '}' {
				templateBraces = templateBraces[:len(templateBraces)-1]
			}
			index++
			part := miniGameJSToken{"", 'x', startLine}
			for index < len(source) {
				current := source[index]
				index++
				if current == '\n' {
					line++
				}
				if current == '\\' && index < len(source) {
					if source[index] == '\n' {
						line++
					}
					index++
					continue
				}
				if current == '`' {
					break
				}
				if current == '$' && index < len(source) && source[index] == '{' {
					index++
					templateBraces = append(templateBraces, 1)
					part = miniGameJSToken{"(", 'p', startLine}
					break
				}
			}
			tokens = append(tokens, part)
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			index++
			var value strings.Builder
			valid, closed := true, false
			for index < len(source) {
				current := source[index]
				index++
				if current == ch {
					closed = true
					break
				}
				if current == '\n' {
					line++
					if ch != '`' {
						valid = false
					}
				}
				if current == '\\' {
					if index >= len(source) {
						break
					}
					escaped := source[index]
					index++
					switch escaped {
					case '\\', '\'', '"', '`':
						value.WriteByte(escaped)
					case 'n':
						value.WriteByte('\n')
					case 'r':
						value.WriteByte('\r')
					case 't':
						value.WriteByte('\t')
					case '\n':
						line++
					case 'u', 'x':
						size := 4
						if escaped == 'x' {
							size = 2
						}
						if index+size <= len(source) {
							code, err := strconv.ParseUint(source[index:index+size], 16, 32)
							if err != nil {
								valid = false
							} else {
								value.WriteRune(rune(code))
							}
							index += size
						} else {
							valid = false
						}
					default:
						value.WriteByte(escaped)
					}
				} else {
					value.WriteByte(current)
				}
			}
			kind := byte('s')
			if ch == '`' || !valid || !closed {
				kind = 'x'
			}
			tokens = append(tokens, miniGameJSToken{value.String(), kind, startLine})
			continue
		}
		if ch == '/' && miniGameRegexAllowed(tokens) {
			index++
			bracket := false
			for index < len(source) {
				current := source[index]
				index++
				if current == '\\' && index < len(source) {
					index++
					continue
				}
				if current == '\n' {
					line++
					break
				}
				if current == '[' {
					bracket = true
				}
				if current == ']' {
					bracket = false
				}
				if current == '/' && !bracket {
					break
				}
			}
			for index < len(source) && miniGameIdentifier(source[index]) {
				index++
			}
			tokens = append(tokens, miniGameJSToken{"", 'x', startLine})
			continue
		}
		if miniGameIdentifier(ch) {
			index++
			for index < len(source) && (miniGameIdentifier(source[index]) || source[index] >= '0' && source[index] <= '9') {
				index++
			}
			kind := byte('i')
			if len(templateBraces) > 0 {
				kind = 't'
			}
			tokens = append(tokens, miniGameJSToken{source[start:index], kind, startLine})
			continue
		}
		if len(templateBraces) > 0 {
			if ch == '{' {
				templateBraces[len(templateBraces)-1]++
			} else if ch == '}' {
				templateBraces[len(templateBraces)-1]--
			}
		}
		index++
		tokens = append(tokens, miniGameJSToken{string(ch), 'p', startLine})
	}
	return tokens
}

func miniGameIdentifier(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == '$'
}

func miniGameRegexAllowed(tokens []miniGameJSToken) bool {
	if len(tokens) == 0 {
		return true
	}
	last := tokens[len(tokens)-1]
	if last.kind == 's' || last.kind == 'x' {
		return false
	}
	switch last.text {
	case "=", "(", "[", "{", ",", ":", ";", "!", "?", "|", "&", "return", "throw", "}":
		return true
	case ">":
		return len(tokens) > 1 && tokens[len(tokens)-2].text == "="
	}
	return false
}
