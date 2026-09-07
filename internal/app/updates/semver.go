package updates

import (
	"regexp"
	"strconv"
	"strings"
)

// parsed is a version the comparison understands: a release tag (v1.4.0), a
// prerelease (v1.5.0-rc.1) or a git describe string (v1.4.0-3-gabc1234, which
// is three commits past v1.4.0 and therefore newer than it).
type parsed struct {
	major, minor, patch int
	pre                 string
	ahead               int
}

// describeSuffix is git describe's "-<n>-g<sha>" tail, with whatever precedes
// it (a prerelease such as rc.1, or nothing) captured first.
var describeSuffix = regexp.MustCompile(`^(?:(.*)-)?(\d+)-g[0-9a-f]+$`)

func parseVersion(raw string) (parsed, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
	s = strings.TrimSuffix(s, "-dirty")
	if s == "" {
		return parsed{}, false
	}
	core, rest, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return parsed{}, false
	}
	var p parsed
	nums := []*int{&p.major, &p.minor, &p.patch}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return parsed{}, false
		}
		*nums[i] = n
	}
	if rest != "" {
		if m := describeSuffix.FindStringSubmatch(rest); m != nil {
			ahead, err := strconv.Atoi(m[2])
			if err != nil {
				return parsed{}, false
			}
			p.pre = m[1]
			p.ahead = ahead
		} else {
			p.pre = rest
		}
	}
	return p, true
}

// compare orders two parsed versions: -1 when a < b, 0 when equal, 1 when a > b.
func compare(a, b parsed) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	// A prerelease sorts before its release; commits past a tag sort after it.
	switch {
	case a.pre != "" && b.pre == "":
		return -1
	case a.pre == "" && b.pre != "":
		return 1
	case a.pre != b.pre:
		if a.pre < b.pre {
			return -1
		}
		return 1
	}
	if a.ahead != b.ahead {
		if a.ahead < b.ahead {
			return -1
		}
		return 1
	}
	return 0
}

// newer reports whether latest is a newer version than running. The second
// result is false when either side cannot be parsed (a "dev" build), in which
// case the caller has to fall back to the checkout's commit distance.
func newer(latest, running string) (isNewer, comparable bool) {
	l, ok := parseVersion(latest)
	if !ok {
		return false, false
	}
	r, ok := parseVersion(running)
	if !ok {
		return false, false
	}
	return compare(l, r) > 0, true
}
