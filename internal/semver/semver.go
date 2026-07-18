package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

type Version struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

func Parse(raw string) (Version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	parts := make([]uint64, 3)
	for index := range parts {
		value, err := strconv.ParseUint(matches[index+1], 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("invalid semantic version %q: %w", raw, err)
		}
		parts[index] = value
	}
	var prerelease []string
	if matches[4] != "" {
		prerelease = strings.Split(matches[4], ".")
	}
	return Version{major: parts[0], minor: parts[1], patch: parts[2], prerelease: prerelease}, nil
}

func Compare(left, right string) (int, error) {
	lhs, err := Parse(left)
	if err != nil {
		return 0, err
	}
	rhs, err := Parse(right)
	if err != nil {
		return 0, err
	}
	return lhs.Compare(rhs), nil
}

func (left Version) Compare(right Version) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(left.prerelease) && index < len(right.prerelease); index++ {
		comparison := compareIdentifier(left.prerelease[index], right.prerelease[index])
		if comparison != 0 {
			return comparison
		}
	}
	switch {
	case len(left.prerelease) < len(right.prerelease):
		return -1
	case len(left.prerelease) > len(right.prerelease):
		return 1
	default:
		return 0
	}
}

func Satisfies(version, constraints string) (bool, error) {
	parsed, err := Parse(version)
	if err != nil {
		return false, err
	}
	fields := strings.Fields(constraints)
	if len(fields) == 0 {
		return false, fmt.Errorf("empty semantic version constraint")
	}
	for _, field := range fields {
		operator, rawVersion := splitConstraint(field)
		required, err := Parse(rawVersion)
		if err != nil {
			return false, fmt.Errorf("invalid constraint %q: %w", field, err)
		}
		comparison := parsed.Compare(required)
		matches := false
		switch operator {
		case ">=":
			matches = comparison >= 0
		case "<=":
			matches = comparison <= 0
		case ">":
			matches = comparison > 0
		case "<":
			matches = comparison < 0
		case "=", "":
			matches = comparison == 0
		default:
			return false, fmt.Errorf("unsupported semantic version operator %q", operator)
		}
		if !matches {
			return false, nil
		}
	}
	return true, nil
}

func splitConstraint(value string) (string, string) {
	for _, operator := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(value, operator) {
			return operator, strings.TrimPrefix(value, operator)
		}
	}
	return "", value
}

func compareIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.ParseUint(left, 10, 64)
	rightNumber, rightErr := strconv.ParseUint(right, 10, 64)
	switch {
	case leftErr == nil && rightErr == nil:
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
