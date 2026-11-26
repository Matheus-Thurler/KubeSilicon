// Package sandbox wraps a command in sandbox-exec: writes only under rw (plus the Metal shader
// caches), no reads of the home directory, /tmp or external volumes except under rw and ro, and
// no way to ask another process to act outside it (unix sockets such as Docker's, signals, Apple
// events, "open", launchd, preferences, pasteboard). The IP network stays open (jobs may fetch
// datasets). It narrows what a mistake can touch; it is not a boundary against hostile code.
// ponytail: sandbox-exec is deprecated by Apple but works; this is the only place that knows it.
package sandbox

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const rules = `(version 1)
(allow default)
(deny file-write*)
(allow file-write* (subpath "/dev"))
(deny file-read* (subpath "/private/tmp") (subpath "/Volumes"))
(allow file-read-metadata (subpath "/private/tmp"))
(deny network-outbound (remote unix-socket))
(allow network-outbound (literal "/private/var/run/mDNSResponder"))
(deny signal)
(allow signal (target same-sandbox))
(deny appleevent-send)
(deny lsopen)
(deny job-creation)
(deny user-preference-write)
(deny mach-lookup (global-name "com.apple.pasteboard.1"))
`

// Profile returns an SBPL profile. Later rules win, so the allows follow the denies.
func Profile(home string, rw, ro []string) string {
	var b strings.Builder
	b.WriteString(rules)
	// Metal writes its compiled shaders to the per-user cache and temp directories.
	for _, c := range [][2]string{
		{"DARWIN_USER_CACHE_DIR", "com.apple.metal"},
		{"DARWIN_USER_CACHE_DIR", "com.apple.metalfe"},
		{"DARWIN_USER_TEMP_DIR", "com.apple.MetalPerformanceShadersGraph"},
	} {
		if out, err := exec.Command("/usr/bin/getconf", c[0]).Output(); err == nil {
			p := filepath.Join(real(strings.TrimSpace(string(out))), c[1])
			fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", p)
		}
	}
	fmt.Fprintf(&b, "(deny file-read* (subpath %q))\n", home)
	// Metadata only (stat, path traversal): exec needs it on every parent directory. No contents, no listings.
	fmt.Fprintf(&b, "(allow file-read-metadata (subpath %q))\n", home)
	for _, p := range rw {
		fmt.Fprintf(&b, "(allow file-read* file-write* network-outbound (subpath %q))\n", real(p))
	}
	for _, p := range ro {
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", real(p))
	}
	return b.String()
}

// real resolves symlinks: the sandbox matches the resolved path (/tmp is /private/tmp).
func real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// Argv prefixes argv with sandbox-exec and a profile file.
func Argv(profileFile string, argv []string) []string {
	return append([]string{"/usr/bin/sandbox-exec", "-f", profileFile}, argv...)
}
