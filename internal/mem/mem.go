// Package mem reads the macOS memory numbers the job queue needs.
package mem

/*
#include <libproc.h>
#include <sys/resource.h>
*/
import "C"

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// Footprint returns the physical footprint, in bytes, of every process in process group pgid.
// It is the "Memory" of Activity Monitor. RSS is not usable here: it leaves Metal (GPU)
// allocations out, so a job holding 4 GiB on the GPU showed an RSS of 19 MB.
func Footprint(pgid int) uint64 {
	pids := make([]C.int, 1024)
	n := int(C.proc_listpgrppids(C.pid_t(pgid), unsafe.Pointer(&pids[0]), C.int(len(pids)*C.sizeof_int)))
	var sum uint64
	for i := 0; i < n && i < len(pids); i++ {
		var ri C.struct_rusage_info_v2
		if C.proc_pid_rusage(pids[i], C.RUSAGE_INFO_V2, (*C.rusage_info_t)(unsafe.Pointer(&ri))) == 0 {
			sum += uint64(ri.ri_phys_footprint)
		}
	}
	return sum
}

// Available returns the memory the kernel reports as available (the number behind the memory
// pressure gauge) and the physical total, in bytes.
// ponytail: one noisy sample, 1% steps. Good enough to decide whether a job may start; the
// per-job limit is what is enforced. Average several samples if admission flaps.
func Available() (avail, total uint64) {
	total, _ = unix.SysctlUint64("hw.memsize")
	level, _ := unix.SysctlUint32("kern.memorystatus_level")
	return total / 100 * uint64(level), total
}
