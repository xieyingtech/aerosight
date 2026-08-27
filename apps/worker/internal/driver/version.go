package driver

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
}

func ParseVersion(raw string) (Version, error) {
	matches := semverPattern.FindStringSubmatch(raw)
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	prerelease := ""
	if dash := strings.IndexByte(raw, '-'); dash >= 0 {
		end := strings.IndexByte(raw[dash:], '+')
		if end < 0 {
			prerelease = raw[dash+1:]
		} else {
			prerelease = raw[dash+1 : dash+end]
		}
	}
	return Version{Major: major, Minor: minor, Patch: patch, Prerelease: prerelease}, nil
}

func (version Version) Compare(other Version) int {
	left := []int{version.Major, version.Minor, version.Patch}
	right := []int{other.Major, other.Minor, other.Patch}
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if version.Prerelease == other.Prerelease {
		return 0
	}
	if version.Prerelease == "" {
		return 1
	}
	if other.Prerelease == "" {
		return -1
	}
	return strings.Compare(version.Prerelease, other.Prerelease)
}

type versionPredicate func(Version) bool

type VersionConstraint struct {
	raw        string
	predicates []versionPredicate
}

func (constraint VersionConstraint) Matches(version Version) bool {
	for _, predicate := range constraint.predicates {
		if !predicate(version) {
			return false
		}
	}
	return true
}

func ParseVersionConstraint(raw string) (VersionConstraint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "*" {
		return VersionConstraint{raw: raw}, nil
	}
	parts := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	constraint := VersionConstraint{raw: raw}
	for _, part := range parts {
		predicate, err := parseVersionPredicate(part)
		if err != nil {
			return VersionConstraint{}, err
		}
		constraint.predicates = append(constraint.predicates, predicate)
	}
	return constraint, nil
}

func parseVersionPredicate(raw string) (versionPredicate, error) {
	operator := "="
	value := raw
	for _, candidate := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(raw, candidate) {
			operator = candidate
			value = strings.TrimPrefix(raw, candidate)
			break
		}
	}
	version, err := ParseVersion(value)
	if err != nil {
		return nil, fmt.Errorf("invalid driver version constraint %q: %w", raw, err)
	}
	switch operator {
	case "=":
		return func(candidate Version) bool { return candidate.Compare(version) == 0 }, nil
	case ">=":
		return func(candidate Version) bool { return candidate.Compare(version) >= 0 }, nil
	case "<=":
		return func(candidate Version) bool { return candidate.Compare(version) <= 0 }, nil
	case ">":
		return func(candidate Version) bool { return candidate.Compare(version) > 0 }, nil
	case "<":
		return func(candidate Version) bool { return candidate.Compare(version) < 0 }, nil
	case "^":
		upper := Version{Major: version.Major + 1}
		if version.Major == 0 {
			upper = Version{Minor: version.Minor + 1}
			if version.Minor == 0 {
				upper = Version{Patch: version.Patch + 1}
			}
		}
		return func(candidate Version) bool {
			return candidate.Compare(version) >= 0 && candidate.Compare(upper) < 0
		}, nil
	case "~":
		upper := Version{Major: version.Major, Minor: version.Minor + 1}
		return func(candidate Version) bool {
			return candidate.Compare(version) >= 0 && candidate.Compare(upper) < 0
		}, nil
	default:
		return nil, fmt.Errorf("unsupported driver version constraint %q", raw)
	}
}
