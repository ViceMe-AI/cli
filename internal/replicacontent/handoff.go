package replicacontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var environmentReferencePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:process\.env|import\.meta\.env)\.([A-Z][A-Z0-9_]*)`),
	regexp.MustCompile(`(?i)os\.Getenv\(\s*["']([A-Z][A-Z0-9_]*)["']\s*\)`),
}

func generateProjectHandoff(files []frozenSourceFile, environmentNames map[string]struct{}, options FreezeSourceOptions) ([]byte, error) {
	byName := make(map[string]frozenSourceFile, len(files))
	for _, file := range files {
		byName[file.name] = file
	}
	purpose := strings.Join(strings.Fields(options.Purpose), " ")
	if purpose == "" {
		purpose = "Reproduce and continue development of this website from the frozen source project."
	}
	stack, packageManager, scripts := inspectProjectManifest(byName)
	readmes := matchingRootFiles(byName, func(name string) bool {
		lower := strings.ToLower(name)
		return lower == "readme" || strings.HasPrefix(lower, "readme.")
	})
	directories := matchingRootDirectories(byName)
	entryPoints := matchingEntryPoints(byName)
	environment := make([]string, 0, len(environmentNames))
	for name := range environmentNames {
		environment = append(environment, name)
	}
	sort.Strings(environment)

	var document strings.Builder
	fmt.Fprintf(&document, "%s\n\n%s\n\n", projectHandoffTitle, projectHandoffTrustNotice)
	fmt.Fprintf(&document, "%s\n\n%s\n\n", projectHandoffPurpose, purpose)
	fmt.Fprintf(&document, "%s\n\n- Stack: %s\n- Package manager: %s\n\n", projectHandoffTechnology, stack, packageManager)
	fmt.Fprintf(&document, "%s\n\n- Key directories: %s\n- Entry points: %s\n\n", projectHandoffDirectories, markdownCodeList(directories), markdownCodeList(entryPoints))
	fmt.Fprintf(&document, "%s\n\n- Available scripts: %s\n- README files: %s\n\n", projectHandoffScripts, markdownCodeList(scripts), markdownCodeList(readmes))
	fmt.Fprintf(&document, "%s\n\n", projectHandoffEnvironment)
	if len(environment) == 0 {
		document.WriteString("- None detected.\n\n")
	} else {
		for _, name := range environment {
			fmt.Fprintf(&document, "- `%s`\n", name)
		}
		document.WriteByte('\n')
	}
	fmt.Fprintf(&document, "%s\n\n- This handoff was generated from the frozen file manifest; ViceMe did not execute, build, test, or deploy the project.\n", projectHandoffLimitations)
	if notes := strings.TrimSpace(options.CreatorNotes); notes != "" {
		fmt.Fprintf(&document, "\n%s\n\n%s\n", projectHandoffCreatorNotes, notes)
	}
	data := []byte(document.String())
	if err := validateProjectHandoff(data); err != nil {
		return nil, err
	}
	if err := validateSensitiveContent(ProjectHandoffFile, data); err != nil {
		return nil, err
	}
	if err := validateForbiddenReplicaContent(ProjectHandoffFile, data); err != nil {
		return nil, err
	}
	return data, nil
}

func inspectProjectManifest(files map[string]frozenSourceFile) (string, string, []string) {
	var stack, managers, scripts []string
	if file, found := files["package.json"]; found {
		stack = append(stack, "Node.js")
		data, err := os.ReadFile(file.snapshot)
		if err == nil {
			var manifest struct {
				Scripts         map[string]string `json:"scripts"`
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			}
			if json.Unmarshal(data, &manifest) == nil {
				for _, framework := range []string{"next", "react", "vue", "svelte", "astro", "@angular/core"} {
					if _, ok := manifest.Dependencies[framework]; ok {
						stack = append(stack, framework)
						continue
					}
					if _, ok := manifest.DevDependencies[framework]; ok {
						stack = append(stack, framework)
					}
				}
				for name := range manifest.Scripts {
					scripts = append(scripts, name)
				}
			}
		}
	}
	for filename, technology := range map[string]string{
		"go.mod": "Go", "Cargo.toml": "Rust", "pyproject.toml": "Python", "requirements.txt": "Python",
		"Gemfile": "Ruby", "composer.json": "PHP", "pom.xml": "Java",
	} {
		if _, found := files[filename]; found && !containsString(stack, technology) {
			stack = append(stack, technology)
		}
	}
	for filename, manager := range map[string]string{
		"pnpm-lock.yaml": "pnpm", "yarn.lock": "Yarn", "package-lock.json": "npm", "bun.lock": "Bun", "bun.lockb": "Bun",
	} {
		if _, found := files[filename]; found && !containsString(managers, manager) {
			managers = append(managers, manager)
		}
	}
	if _, found := files["go.mod"]; found {
		managers = append(managers, "Go modules")
	}
	if _, found := files["Cargo.toml"]; found {
		managers = append(managers, "Cargo")
	}
	if len(stack) == 0 {
		stack = append(stack, "No supported stack manifest detected")
	}
	if len(managers) == 0 {
		managers = append(managers, "No lockfile or package manager manifest detected")
	}
	sort.Strings(stack)
	sort.Strings(managers)
	sort.Strings(scripts)
	return strings.Join(stack, ", "), strings.Join(managers, ", "), scripts
}

func collectEnvironmentReferences(data []byte, names map[string]struct{}) {
	if !utf8.Valid(data) {
		return
	}
	for _, pattern := range environmentReferencePatterns {
		for _, match := range pattern.FindAllSubmatch(data, -1) {
			if len(match) == 2 {
				names[string(match[1])] = struct{}{}
			}
		}
	}
	collectEnvironmentFileNames(data, names)
}

func collectEnvironmentFileNames(data []byte, names map[string]struct{}) {
	if !utf8.Valid(data) {
		return
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, found := strings.Cut(line, "=")
		if found && validEnvironmentName(strings.TrimSpace(name)) {
			names[strings.TrimSpace(name)] = struct{}{}
		}
	}
}

func validEnvironmentName(value string) bool {
	if value == "" || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func isEnvironmentFile(base string) bool {
	if base == ".env.example" || base == ".env.sample" || base == ".env.template" {
		return false
	}
	return base == ".env" || strings.HasPrefix(base, ".env.")
}

func matchingRootFiles(files map[string]frozenSourceFile, match func(string) bool) []string {
	var result []string
	for name := range files {
		if !strings.Contains(name, "/") && match(name) {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func matchingRootDirectories(files map[string]frozenSourceFile) []string {
	wanted := map[string]struct{}{"app": {}, "src": {}, "pages": {}, "public": {}, "server": {}, "api": {}, "components": {}, "packages": {}}
	seen := make(map[string]struct{})
	for name := range files {
		root, _, nested := strings.Cut(name, "/")
		if _, found := wanted[root]; nested && found {
			seen[root+"/"] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func matchingEntryPoints(files map[string]frozenSourceFile) []string {
	var result []string
	for name := range files {
		base := strings.ToLower(path.Base(name))
		if base == "index.html" || strings.HasPrefix(base, "index.") || strings.HasPrefix(base, "main.") || strings.HasPrefix(base, "app.") || name == "package.json" {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func markdownCodeList(values []string) string {
	if len(values) == 0 {
		return "None detected"
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = "`" + strings.ReplaceAll(value, "`", "") + "`"
	}
	return strings.Join(quoted, ", ")
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func readEnvironmentFile(filename string, info os.FileInfo) ([]byte, error) {
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > 1<<20 {
		return nil, errors.New("environment file is not a bounded regular file")
	}
	return os.ReadFile(filename)
}
