package version

import (
	"runtime/debug"
	"strings"
)

// These can be overridden at build time via -ldflags:
//
//	go install -ldflags "-X github.com/oksasatya/chasago/internal/version.Version=v0.1.0"
//
// When unset, we fall back to Go's embedded build info (populated automatically
// since Go 1.18 when building from a VCS checkout, and by `go install path@ver`).
var (
	Version = ""
	Commit  = ""
	Date    = ""
)

func String() string {
	v, c, d := resolve()
	return v + " (" + c + ", " + d + ")"
}

func resolve() (ver, commit, date string) {
	ver, commit, date = Version, Commit, Date

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fallback(ver, commit, date)
	}

	if ver == "" {
		ver = info.Main.Version
		if ver == "" || ver == "(devel)" {
			ver = "dev"
		}
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "" && s.Value != "" {
				commit = shortCommit(s.Value)
			}
		case "vcs.time":
			if date == "" && s.Value != "" {
				date = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" && !strings.HasSuffix(commit, "-dirty") {
				commit += "-dirty"
			}
		}
	}
	return fallback(ver, commit, date)
}

func fallback(ver, commit, date string) (string, string, string) {
	if ver == "" {
		ver = "dev"
	}
	if commit == "" {
		commit = "none"
	}
	if date == "" {
		date = "unknown"
	}
	return ver, commit, date
}

func shortCommit(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
