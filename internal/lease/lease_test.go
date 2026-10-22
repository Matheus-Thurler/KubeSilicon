package lease

import "testing"

const sample = `{
	name=vm0
	ip_address=192.168.64.2
	lease=0x6ada576a
}
{
	name=other
	ip_address=192.168.64.9
	lease=0x6fffffff
}
{
	name=vm0
	ip_address=192.168.64.7
	lease=0x6bda576a
}
{
	name=vm0
	ip_address=192.168.64.3
	lease=0x6aaa576a
}
`

func TestParsePicksNewestLease(t *testing.T) {
	if got := parse(sample, "vm0"); got != "192.168.64.7" {
		t.Fatalf("got %q", got)
	}
	if got := parse(sample, "nope"); got != "" {
		t.Fatalf("got %q", got)
	}
}
