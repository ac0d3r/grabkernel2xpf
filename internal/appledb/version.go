package appledb

import (
	"fmt"
	"sort"
	"strings"
)

// VersionRange is an inclusive release span.
//
// How many components the upper bound names decides what it covers:
// a major includes every minor and patch of that major, a major.minor
// includes every patch of that minor, and a full version is exact.
type VersionRange struct {
	lo          [3]int
	hi          [3]int
	hiPrecision int
}

func ParseVersionRange(spec string) (VersionRange, error) {
	s := strings.TrimSpace(spec)
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return VersionRange{}, fmt.Errorf("empty version")
	}
	loSpec, hiSpec, ok := strings.Cut(s, "-")
	if !ok {
		hiSpec = loSpec
	} else if loSpec == "" || hiSpec == "" || strings.Contains(hiSpec, "-") {
		return VersionRange{}, fmt.Errorf("invalid version %q", spec)
	}
	loN, _, err := parseBound(loSpec)
	if err != nil {
		return VersionRange{}, err
	}
	hiN, hiPrec, err := parseBound(hiSpec)
	if err != nil {
		return VersionRange{}, err
	}
	r := VersionRange{lo: loN, hi: hiN, hiPrecision: hiPrec}
	if r.reversed() {
		return VersionRange{}, fmt.Errorf("version range %q is reversed", spec)
	}
	return r, nil
}

func (r VersionRange) reversed() bool {
	switch r.hiPrecision {
	case 1:
		return r.lo[0] > r.hi[0]
	case 2:
		if r.lo[0] != r.hi[0] {
			return r.lo[0] > r.hi[0]
		}
		return r.lo[1] > r.hi[1]
	default:
		return cmpVer(r.lo, r.hi) > 0
	}
}

func parseBound(s string) (n [3]int, precision int, err error) {
	if s == "" {
		return n, 0, fmt.Errorf("empty version")
	}
	orig := s
	var ok bool
	n[0], s, ok = nextInt(s)
	if !ok {
		return n, 0, fmt.Errorf("invalid version %q", orig)
	}
	precision = 1
	for i := 1; i < 3; i++ {
		if s == "" {
			return n, precision, nil
		}
		if s[0] != '.' {
			return n, 0, fmt.Errorf("invalid version %q", orig)
		}
		n[i], s, ok = nextInt(s[1:])
		if !ok {
			return n, 0, fmt.Errorf("invalid version %q", orig)
		}
		precision++
	}
	if s != "" {
		return n, 0, fmt.Errorf("invalid version %q", orig)
	}
	return n, precision, nil
}

func nextInt(s string) (n int, rest string, ok bool) {
	n, rest = readInt(s)
	return n, rest, rest != s
}

func (r VersionRange) contains(major, minor, patch int) bool {
	if cmpVer([3]int{major, minor, patch}, r.lo) < 0 {
		return false
	}
	switch r.hiPrecision {
	case 1:
		return major <= r.hi[0]
	case 2:
		if major != r.hi[0] {
			return major < r.hi[0]
		}
		return minor <= r.hi[1]
	default:
		return cmpVer([3]int{major, minor, patch}, r.hi) <= 0
	}
}

func VersionInRanges(version string, ranges []VersionRange) bool {
	version = strings.TrimSpace(version)
	if version == "" || len(ranges) == 0 {
		return false
	}
	k := parseVersion(version, false, false)
	if k.channel != 0 {
		return false
	}
	for _, r := range ranges {
		if r.contains(k.major, k.minor, k.patch) {
			return true
		}
	}
	return false
}

func CompareVersions(a, b string) int {
	ka := parseVersion(a, false, false)
	kb := parseVersion(b, false, false)
	return cmpVer([3]int{ka.major, ka.minor, ka.patch}, [3]int{kb.major, kb.minor, kb.patch})
}

func cmpVer(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// SelectReleases keeps final releases of osName that fall in any range,
// oldest version first.
func SelectReleases(builds []Build, osName string, ranges []VersionRange) []Build {
	var out []Build
	for _, b := range builds {
		if b.Build == "" || b.Beta || b.RC {
			continue
		}
		if !osMatch(b.OS, osName) {
			continue
		}
		if !VersionInRanges(b.Version, ranges) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		return releaseLess(out[i], out[j])
	})
	return out
}

func osMatch(buildOS, want string) bool {
	if buildOS == "" || strings.EqualFold(buildOS, want) {
		return true
	}
	// AppleDB labels some iPad firmware as iOS.
	return strings.EqualFold(want, "iPadOS") && strings.EqualFold(buildOS, "iOS")
}

func releaseLess(a, b Build) bool {
	c := CompareVersions(a.Version, b.Version)
	if c != 0 {
		return c < 0
	}
	return a.Build < b.Build
}
