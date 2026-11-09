// Package sandbox wraps a command in sandbox-exec: writes only under rw, no reads of the home
// directory except under rw and ro. Network stays open (jobs may fetch datasets).
// ponytail: sandbox-exec is deprecated by Apple but works; this is the only place that knows it.
package sandbox

import (
	"fmt"
	"strings"
)

// Profile returns an SBPL profile. Later rules win, so the allows follow the denies.
func Profile(home string, rw, ro []string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny file-write*)\n")
	b.WriteString(`(allow file-write* (subpath "/private/var/folders") (subpath "/dev"))` + "\n")
	fmt.Fprintf(&b, "(deny file-read* (subpath %q))\n", home)
	// Metadata only (stat, path traversal): exec needs it on every parent directory. No contents, no listings.
	fmt.Fprintf(&b, "(allow file-read-metadata (subpath %q))\n", home)
	for _, p := range rw {
		fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", p)
	}
	for _, p := range ro {
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", p)
	}
	return b.String()
}

// Argv prefixes argv with sandbox-exec and a profile file.
func Argv(profileFile string, argv []string) []string {
	return append([]string{"/usr/bin/sandbox-exec", "-f", profileFile}, argv...)
}
