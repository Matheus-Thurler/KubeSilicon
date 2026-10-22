// Package lease finds a VM's IP in the macOS NAT DHCP leases.
package lease

import (
	"os"
	"strconv"
	"strings"
)

const File = "/var/db/dhcpd_leases"

// IP returns the ip_address of the newest lease with name=hostname, or "".
// Hostnames must be unique per VM instance: the file keeps stale leases from
// old VMs, and a restarted VM reuses its lease without renewing the expiry.
func IP(path, hostname string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return parse(string(b), hostname)
}

func parse(s, hostname string) (ip string) {
	var best uint64
	for _, blk := range strings.Split(s, "}") {
		var name, addr string
		var exp uint64
		for _, l := range strings.Split(blk, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
			if !ok {
				continue
			}
			switch k {
			case "name":
				name = v
			case "ip_address":
				addr = v
			case "lease":
				exp, _ = strconv.ParseUint(strings.TrimPrefix(v, "0x"), 16, 64)
			}
		}
		if name == hostname && addr != "" && exp >= best {
			best, ip = exp, addr
		}
	}
	return ip
}
