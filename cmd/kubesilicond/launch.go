package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Matheus-Thurler/kubesilicon/internal/vm"
)

// launch is the per-VM process ("kubesilicond launch ..."). It owns one VM, so a
// crash or restart of the daemon never takes a running VM down.
func launch(args []string) {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	var c vm.Config
	var cpus uint
	fs.StringVar(&c.Disk, "disk", "", "raw disk")
	fs.StringVar(&c.Seed, "seed", "", "cloud-init ISO")
	fs.StringVar(&c.EFIVars, "efi", "", "EFI variable store")
	fs.StringVar(&c.Console, "console", "", "serial output file")
	fs.StringVar(&c.Sock, "sock", "", "console unix socket")
	fs.UintVar(&cpus, "cpus", 2, "vCPUs")
	fs.Uint64Var(&c.MemBytes, "mem", 2<<30, "memory in bytes")
	fs.Parse(args)
	c.CPUs = cpus

	signal.Ignore(syscall.SIGHUP)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	v, err := vm.Start(c)
	if err != nil {
		log.Fatal(err)
	}
	select {
	case <-sig:
		v.Stop(20 * time.Second)
	case <-v.Done:
	}
}
