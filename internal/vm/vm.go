// Package vm wraps Virtualization.framework (via vz) for a Linux ARM64 VM.
package vm

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Code-Hex/vz/v3"
)

type Config struct {
	Disk, Seed, EFIVars string
	Console             string // serial output is appended here
	Sock                string // unix socket for an interactive serial console (one client at a time)
	CPUs                uint
	MemBytes            uint64
}

type VM struct {
	vm   *vz.VirtualMachine
	Done chan struct{} // closed when the VM stops (guest shutdown, error or Stop)

	files []io.Closer
	ln    net.Listener

	mu     sync.Mutex
	client net.Conn // current console client, if any
}

func Start(c Config) (*VM, error) {
	store, err := vz.NewEFIVariableStore(c.EFIVars, vz.WithCreatingEFIVariableStore())
	if err != nil {
		return nil, err
	}
	boot, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(store))
	if err != nil {
		return nil, err
	}
	cfg, err := vz.NewVirtualMachineConfiguration(boot, c.CPUs, c.MemBytes)
	if err != nil {
		return nil, err
	}
	plat, err := vz.NewGenericPlatformConfiguration()
	if err != nil {
		return nil, err
	}
	cfg.SetPlatformVirtualMachineConfiguration(plat)

	var devs []vz.StorageDeviceConfiguration
	for _, d := range []struct {
		path string
		ro   bool
	}{{c.Disk, false}, {c.Seed, true}} {
		// Cached is required: with the default or uncached mode, Linux guests on an M5 Pro
		// (macOS 27) hit random kernel Oops and SIGILL in 3 of 4 boots (see docs/validation.md).
		att, err := vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync(d.path, d.ro, vz.DiskImageCachingModeCached, vz.DiskImageSynchronizationModeFsync)
		if err != nil {
			return nil, err
		}
		blk, err := vz.NewVirtioBlockDeviceConfiguration(att)
		if err != nil {
			return nil, err
		}
		devs = append(devs, blk)
	}
	cfg.SetStorageDevicesVirtualMachineConfiguration(devs)

	nat, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, err
	}
	nic, err := vz.NewVirtioNetworkDeviceConfiguration(nat)
	if err != nil {
		return nil, err
	}
	mac, err := vz.NewRandomLocallyAdministeredMACAddress()
	if err != nil {
		return nil, err
	}
	nic.SetMACAddress(mac)
	cfg.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{nic})

	v := &VM{Done: make(chan struct{})}
	fail := func(err error) (*VM, error) { v.closeFiles(); return nil, err }

	logf, err := os.OpenFile(c.Console, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	v.files = append(v.files, logf)
	inR, inW, err := os.Pipe() // client -> guest
	if err != nil {
		return fail(err)
	}
	outR, outW, err := os.Pipe() // guest -> log file and client
	if err != nil {
		inR.Close()
		inW.Close()
		return fail(err)
	}
	v.files = append(v.files, inR, inW, outR, outW)
	os.Remove(c.Sock)
	if v.ln, err = net.Listen("unix", c.Sock); err != nil {
		return fail(err)
	}
	if err := os.Chmod(c.Sock, 0o600); err != nil {
		return fail(err)
	}
	go v.serve(inW)
	go v.pump(outR, logf)

	ser, err := vz.NewFileHandleSerialPortAttachment(inR, outW)
	if err != nil {
		return fail(err)
	}
	con, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(ser)
	if err != nil {
		return fail(err)
	}
	cfg.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{con})

	ent, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return fail(err)
	}
	cfg.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{ent})

	if ok, err := cfg.Validate(); !ok || err != nil {
		return fail(fmt.Errorf("invalid config: %v", err))
	}
	v.vm, err = vz.NewVirtualMachine(cfg)
	if err != nil {
		return fail(err)
	}
	if err := v.vm.Start(); err != nil {
		return fail(err)
	}
	go func() {
		for s := range v.vm.StateChangedNotify() {
			if s == vz.VirtualMachineStateStopped || s == vz.VirtualMachineStateError {
				break
			}
		}
		v.closeFiles()
		close(v.Done)
	}()
	return v, nil
}

// Stop requests an ACPI shutdown and forces it after timeout.
func (v *VM) Stop(timeout time.Duration) {
	select {
	case <-v.Done:
		return
	default:
	}
	if v.vm.CanRequestStop() {
		if ok, _ := v.vm.RequestStop(); ok {
			select {
			case <-v.Done:
				return
			case <-time.After(timeout):
			}
		}
	}
	_ = v.vm.Stop()
	<-v.Done
}

func (v *VM) closeFiles() {
	if v.ln != nil {
		v.ln.Close()
	}
	v.mu.Lock()
	if v.client != nil {
		v.client.Close()
	}
	v.mu.Unlock()
	for _, f := range v.files {
		f.Close()
	}
}

// serve connects console clients to the guest input. A new client replaces the old one.
func (v *VM) serve(guestIn io.Writer) {
	for {
		c, err := v.ln.Accept()
		if err != nil {
			return
		}
		v.mu.Lock()
		if v.client != nil {
			v.client.Close()
		}
		v.client = c
		v.mu.Unlock()
		go func() {
			io.Copy(guestIn, c)
			v.mu.Lock()
			if v.client == c {
				v.client = nil
			}
			v.mu.Unlock()
			c.Close()
		}()
	}
}

// pump copies guest output to the log file and to the console client. A slow client is dropped, never waited for.
func (v *VM) pump(guestOut io.Reader, logf io.Writer) {
	buf := make([]byte, 4096)
	for {
		n, err := guestOut.Read(buf)
		if n > 0 {
			logf.Write(buf[:n])
			v.mu.Lock()
			if v.client != nil {
				v.client.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
				if _, werr := v.client.Write(buf[:n]); werr != nil {
					v.client.Close()
					v.client = nil
				}
			}
			v.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}
