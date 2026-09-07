// Package version holds the build identity stamped into every Warmbly binary.
//
// The values are injected at link time (see deploy/docker/*.Dockerfile and the
// Makefile), so a binary knows which release or commit it was built from
// without reading the checkout. .git is excluded from the docker build context,
// which is why runtime/debug's vcs data cannot be used instead.
package version

import (
	"os"
	"strings"
)

var (
	// Version is the release tag (v1.4.0) or a git describe string
	// (v1.4.0-3-gabc1234). Empty when the build did not stamp one.
	Version = ""
	// Commit is the full git sha the binary was built from.
	Commit = ""
	// BuiltAt is the RFC 3339 build time.
	BuiltAt = ""
)

// String is the version to display: the stamped value, WARMBLY_VERSION from
// the environment (the pre-existing override), or "dev".
func String() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("WARMBLY_VERSION")); v != "" {
		return v
	}
	return "dev"
}

// ShortCommit is the first 12 characters of the commit, or empty.
func ShortCommit() string {
	c := strings.TrimSpace(Commit)
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// Info is the JSON shape every surface reports the running build as.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	BuiltAt string `json:"built_at,omitempty"`
}

// Current returns the running build's identity.
func Current() Info {
	return Info{Version: String(), Commit: strings.TrimSpace(Commit), BuiltAt: strings.TrimSpace(BuiltAt)}
}
