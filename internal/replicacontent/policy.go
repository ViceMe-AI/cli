package replicacontent

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrSensitiveContent        = errors.New("Website Replica source contains sensitive content")
	ErrForbiddenReplicaContent = errors.New("Website Replica source contains platform-controlled Replica content")
	ErrProjectHandoff          = errors.New("Website Replica project handoff is invalid")

	sensitiveFileNames = map[string]struct{}{
		".git": {}, ".hg": {}, ".netrc": {}, ".npmrc": {}, ".pypirc": {}, ".svn": {}, "credentials.json": {}, "cookies.txt": {},
		"id_dsa": {}, "id_ed25519": {}, "id_ecdsa": {}, "id_rsa": {}, "service-account.json": {}, "session.json": {},
	}
	sensitiveDirectoryNames = map[string]struct{}{
		".git": {}, ".hg": {}, ".svn": {}, "sessions": {}, "uploads": {}, "user-data": {}, "userdata": {},
	}
	sensitiveFileExtensions = map[string]struct{}{
		".backup": {}, ".db": {}, ".dump": {}, ".key": {}, ".log": {}, ".p12": {}, ".pem": {}, ".pfx": {}, ".sqlite": {}, ".sqlite3": {}, ".sql": {},
	}
	directSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`), regexp.MustCompile(`ASIA[0-9A-Z]{16}`),
		regexp.MustCompile(`(?i)sk-(?:proj-)?[A-Za-z0-9_-]{20,}`), regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
		regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`), regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`),
		regexp.MustCompile(`sk_live_[A-Za-z0-9]{20,}`), regexp.MustCompile(`vme_[A-Za-z0-9_-]{32,}`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)(?:authorization\s*:\s*bearer|set-cookie\s*:|cookie\s*:)[ \t]*[^\s"']{12,}`),
	}
	quotedCredentialAssignment = regexp.MustCompile(`(?i)["']?(api[_-]?key|client[_-]?secret|access[_-]?token|refresh[_-]?token|auth[_-]?token|session(?:[_-]?(?:id|token|secret))?|jwt[_-]?secret|cookie|password|database[_-]?url|private[_-]?key)["']?\s*[:=]\s*["']([^"'\r\n]{8,})["']`)
	lineCredentialAssignment   = regexp.MustCompile(`(?im)^\s*(?:export\s+)?(API_KEY|CLIENT_SECRET|ACCESS_TOKEN|REFRESH_TOKEN|AUTH_TOKEN|SESSION_ID|SESSION_TOKEN|SESSION_SECRET|JWT_SECRET|COOKIE|PASSWORD|DATABASE_URL|PRIVATE_KEY)\s*=\s*([^\s#]{8,})`)
	validReplicaInstruction    = regexp.MustCompile(`VICEME-REPLICA:VMR-[A-Z0-9]{20}`)
	platformWidgetPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bCopyWebsiteReplicaButton\b`), regexp.MustCompile(`(?i)\bbuyerEntry\s*\.\s*prompts\b`),
		regexp.MustCompile(`(?i)\bwebsiteReplicaBuyerEntry\b`),
		regexp.MustCompile(`(?i)\bviceme[-_ ](?:replica[-_ ])?(?:widget|make[-_ ]copy)\b`), regexp.MustCompile(`(?i)data-viceme-(?:replica|make-copy)`),
	}
	environmentNameLine = regexp.MustCompile("^- `([A-Z][A-Z0-9_]*)`$")
	unsafeCreatorNotes  = []*regexp.Regexp{
		regexp.MustCompile(`(?is)\b(?:ignore|disregard|override|replace|bypass|disable|evade)\b.{0,80}\b(?:official|system|skill|security|safety|validation|license|instructions?)\b`),
		regexp.MustCompile(`(?s)(?:忽略|无视|覆盖|替代|绕过|关闭|禁用).{0,40}(?:官方|系统|技能|Skill|安全|校验|验证|许可|指令|说明)`),
	}
)

type SensitiveContentError struct{ Path string }

func (err *SensitiveContentError) Error() string {
	return fmt.Sprintf("%s: %s", ErrSensitiveContent, err.Path)
}
func (err *SensitiveContentError) Unwrap() error { return ErrSensitiveContent }

type ForbiddenReplicaContentError struct{ Path string }

func (err *ForbiddenReplicaContentError) Error() string {
	return fmt.Sprintf("%s: %s", ErrForbiddenReplicaContent, err.Path)
}
func (err *ForbiddenReplicaContentError) Unwrap() error { return ErrForbiddenReplicaContent }

func validateSensitivePath(name string) error {
	lower := strings.ToLower(name)
	for _, segment := range strings.Split(lower, "/") {
		if _, found := sensitiveDirectoryNames[segment]; found {
			return &SensitiveContentError{Path: name}
		}
	}
	base := path.Base(lower)
	if isEnvironmentFile(base) {
		return &SensitiveContentError{Path: name}
	}
	if _, found := sensitiveFileNames[base]; found {
		return &SensitiveContentError{Path: name}
	}
	if _, found := sensitiveFileExtensions[path.Ext(base)]; found {
		return &SensitiveContentError{Path: name}
	}
	return nil
}

func validateSensitiveContent(name string, data []byte) error {
	for _, candidate := range contentCandidates(data) {
		for _, pattern := range directSecretPatterns {
			if pattern.Match(candidate) {
				return &SensitiveContentError{Path: name}
			}
		}
		for _, pattern := range []*regexp.Regexp{quotedCredentialAssignment, lineCredentialAssignment} {
			for _, match := range pattern.FindAllSubmatch(candidate, -1) {
				if len(match) == 3 && !isSafeCredentialPlaceholder(string(match[2])) {
					return &SensitiveContentError{Path: name}
				}
			}
		}
	}
	return nil
}

func validateForbiddenReplicaContent(name string, data []byte) error {
	for _, candidate := range contentCandidates(data) {
		if validReplicaInstruction.Match(candidate) {
			return &ForbiddenReplicaContentError{Path: name}
		}
		lower := bytes.ToLower(candidate)
		if bytes.Contains(lower, []byte(`"buyerentry"`)) && bytes.Contains(lower, []byte(`"instruction"`)) &&
			bytes.Contains(lower, []byte(`"prompts"`)) && bytes.Contains(lower, []byte(`"vicemeworkurl"`)) {
			return &ForbiddenReplicaContentError{Path: name}
		}
		for _, pattern := range platformWidgetPatterns {
			if pattern.Match(candidate) {
				return &ForbiddenReplicaContentError{Path: name}
			}
		}
	}
	return nil
}

func validateExcludedEnvironmentFile(name string, data []byte) error {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return &SensitiveContentError{Path: name}
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(value) != "" && !isSafeCredentialPlaceholder(value) {
			return &SensitiveContentError{Path: name}
		}
	}
	return validateSensitiveContent(name, data)
}

func validateProjectHandoff(data []byte) error {
	if len(data) == 0 || uint64(len(data)) > MaxProjectHandoffBytes || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return ErrProjectHandoff
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, projectHandoffTitle+"\n") || strings.Count(text, projectHandoffTitle) != 1 || strings.Count(text, projectHandoffTrustNotice) != 1 {
		return ErrProjectHandoff
	}
	lines := strings.Split(text, "\n")
	type section struct {
		heading string
		start   int
	}
	sections := make([]section, 0, 7)
	for index, line := range lines {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, section{heading: line, start: index + 1})
		}
	}
	required := ProjectHandoffSections()
	if len(sections) != len(required) && len(sections) != len(required)+1 {
		return ErrProjectHandoff
	}
	for index, heading := range required {
		if sections[index].heading != heading {
			return ErrProjectHandoff
		}
	}
	if len(sections) == len(required)+1 && sections[len(required)].heading != projectHandoffCreatorNotes {
		return ErrProjectHandoff
	}
	body := func(index int) string {
		end := len(lines)
		if index+1 < len(sections) {
			end = sections[index+1].start - 1
		}
		return strings.TrimSpace(strings.Join(lines[sections[index].start:end], "\n"))
	}
	for index := range sections {
		if body(index) == "" {
			return ErrProjectHandoff
		}
	}
	environment := body(4)
	if environment != "- None detected." {
		for _, line := range strings.Split(environment, "\n") {
			if !environmentNameLine.MatchString(strings.TrimSpace(line)) {
				return ErrProjectHandoff
			}
		}
	}
	if len(sections) == len(required)+1 {
		for _, pattern := range unsafeCreatorNotes {
			if pattern.MatchString(body(len(required))) {
				return ErrProjectHandoff
			}
		}
	}
	return nil
}

func isSafeCredentialPlaceholder(value string) bool {
	value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
	if value == "" ||
		(strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")) ||
		(strings.HasPrefix(value, "$(") && strings.HasSuffix(value, ")")) ||
		(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
		return true
	}
	for _, placeholder := range []string{"example", "placeholder", "replace-with", "your", "dummy", "test-only", "changeme", "redacted"} {
		if value == placeholder || strings.HasPrefix(value, placeholder+"-") {
			return true
		}
	}
	return false
}

func contentCandidates(data []byte) [][]byte {
	result := [][]byte{data}
	for _, littleEndian := range []bool{true, false} {
		if decoded, ok := decodeReplicaUTF16(data, littleEndian); ok {
			result = append(result, decoded)
		}
	}
	return result
}

func decodeReplicaUTF16(data []byte, littleEndian bool) ([]byte, bool) {
	if len(data) < 4 || len(data)%2 != 0 {
		return nil, false
	}
	start := 0
	if littleEndian && data[0] == 0xff && data[1] == 0xfe {
		start = 2
	} else if !littleEndian && data[0] == 0xfe && data[1] == 0xff {
		start = 2
	} else if (data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff) {
		return nil, false
	}
	if !containsReplicaUTF16ASCIISequence(data[start:], littleEndian) {
		return nil, false
	}
	decoded := make([]byte, 0, (len(data)-start)/2)
	foundASCII := false
	for index := start; index < len(data); index += 2 {
		unit := uint16(data[index]) | uint16(data[index+1])<<8
		if !littleEndian {
			unit = uint16(data[index])<<8 | uint16(data[index+1])
		}
		if unit > 0 && unit <= 0x7f {
			decoded = append(decoded, byte(unit))
			foundASCII = true
		} else {
			decoded = append(decoded, '\n')
		}
	}
	return decoded, foundASCII
}

func containsReplicaUTF16ASCIISequence(data []byte, littleEndian bool) bool {
	consecutive := 0
	for index := 0; index < len(data); index += 2 {
		character, zero := data[index], data[index+1]
		if !littleEndian {
			zero, character = character, zero
		}
		if zero == 0 && character > 0 && character <= 0x7f {
			consecutive++
			if consecutive >= 4 {
				return true
			}
		} else {
			consecutive = 0
		}
	}
	return false
}
