// Package version is the polyapi binary semver (independent of the delegate protocol).
package version

import (
	"runtime"
	"runtime/debug"
	"time"
)

// Version is the binary semver. Releases override it at link time:
//
//	-X github.com/polyapi/polyglot/src/version.Version=0.2.0
var Version = "0.1.0"

// Commit is the git SHA. Releases override it at link time.
var Commit = ""

// Date is the build timestamp (RFC3339). Releases override it at link time.
var Date = ""

// Info is binary identity plus build metadata.
type Info struct {
	Version string
	Commit  string
	Date    string
	Go      string
	OS      string
	Arch    string
	Dirty   bool
}

// Current returns the running binary's version metadata.
//
// Link-time Version/Commit/Date win over embedded VCS info from
// runtime/debug so tagged dist builds stay exact.
func Current() Info {
	info := Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
	fillFromVCS(&info)
	return info
}

func fillFromVCS(info *Info) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
}

// Platform is GOOS/GOARCH.
func (i Info) Platform() string {
	return i.OS + "/" + i.Arch
}

// CommitDisplay is a short SHA, with -dirty when the tree was modified.
func (i Info) CommitDisplay() string {
	c := i.Commit
	if c == "" {
		return "unknown"
	}
	if len(c) > 12 {
		c = c[:12]
	}
	if i.Dirty {
		return c + "-dirty"
	}
	return c
}

// DateDisplay is the build time in RFC3339 UTC when parseable.
func (i Info) DateDisplay() string {
	if i.Date == "" {
		return "unknown"
	}
	if t, err := time.Parse(time.RFC3339, i.Date); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339Nano, i.Date); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return i.Date
}
