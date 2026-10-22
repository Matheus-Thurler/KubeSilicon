// Package vm wraps Virtualization.framework (via vz) for a Linux ARM64 VM.
package vm

import (
	"fmt"
	"os"
	"time"

	"github.com/Code-Hex/vz/v3"
)

type Config struct {
	Disk, Seed, EFIVars, Console string
	CPUs                         uint
	MemBytes                     uint64
}

type VM struct {
	vm   *vz.VirtualMachine
	Done chan struct{} // closed when the VM stops (guest shutdown, error or Stop)
	in   *os.File      // serial input; the write end is never used
	inW  *os.File
	out  *os.File
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
		att, err := vz.NewDiskImageStorageDeviceAttachment(d.path, d.ro)
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

	out, err := os.OpenFile(c.Console, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	in, inW, err := os.Pipe()
	if err != nil {
		out.Close()
		return nil, err
	}
	v := &VM{Done: make(chan struct{}), in: in, inW: inW, out: out}
	fail := func(err error) (*VM, error) { v.closeFiles(); return nil, err }

	ser, err := vz.NewFileHandleSerialPortAttachment(in, out)
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
	v.in.Close()
	v.inW.Close()
	v.out.Close()
}
