// kubesilicon: client for SiliconVMs. Talks to the cluster through kubectl and to the VM through
// the local console socket and ssh.
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const usage = `usage: kubesilicon [-context C] [-n NAMESPACE] <command> NAME [args]

  ip NAME                  print the VM IP
  ssh NAME [-- ssh args]   ssh to the VM (default user ubuntu, see -l)
  console NAME             serial console, leave with Ctrl-]

flags:
`

func main() {
	kctx := flag.String("context", os.Getenv("KUBESILICON_CONTEXT"), "kubeconfig context (or $KUBESILICON_CONTEXT, required)")
	ns := flag.String("n", "", "namespace (default: the one of the context)")
	user := flag.String("l", "ubuntu", "ssh user")
	root := flag.String("root", filepath.Join(os.Getenv("HOME"), ".kubesilicon"), "state directory of kubesilicond")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage); flag.PrintDefaults() }
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 || *kctx == "" {
		flag.Usage()
		os.Exit(2)
	}
	cmd, name, rest := args[0], args[1], args[2:]
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}

	namespace, ip, err := lookup(*kctx, *ns, name)
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "ip":
		if ip == "" {
			fatal(fmt.Errorf("%s/%s has no IP yet", namespace, name))
		}
		fmt.Println(ip)
	case "ssh":
		if ip == "" {
			fatal(fmt.Errorf("%s/%s has no IP yet", namespace, name))
		}
		bin, err := exec.LookPath("ssh")
		if err != nil {
			fatal(err)
		}
		// VM IPs get reused, so host keys would only produce noise on a local lab.
		argv := append([]string{"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR", "-o", "ConnectTimeout=10", *user + "@" + ip}, rest...)
		fatal(syscall.Exec(bin, argv, os.Environ()))
	case "console":
		fatal(console(filepath.Join(*root, "vms", namespace+"_"+name, "console.sock")))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// lookup reads namespace and status.ip with kubectl, so kubeconfig handling stays in one place.
func lookup(kctx, ns, name string) (namespace, ip string, err error) {
	args := []string{"--context", kctx}
	if ns != "" {
		args = append(args, "-n", ns)
	}
	args = append(args, "get", "siliconvm", name, "-o", `jsonpath={.metadata.namespace}{"\t"}{.status.ip}`)
	out, err := exec.Command("kubectl", args...).Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return "", "", fmt.Errorf("kubectl: %s", strings.TrimSpace(string(ee.Stderr)))
	}
	if err != nil {
		return "", "", err
	}
	namespace, ip, _ = strings.Cut(string(out), "\t")
	return namespace, ip, nil
}

// console attaches the terminal to the VM serial socket. Ctrl-] leaves; stdin EOF leaves too.
func console(sock string) error {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return fmt.Errorf("console: %v (is the VM running?)", err)
	}
	defer c.Close()
	if saved, err := stty("-g"); err == nil { // only when stdin is a terminal
		defer stty(saved)
		stty("raw", "-echo")
		fmt.Fprint(os.Stderr, "connected, Ctrl-] to leave\r\n")
	}
	done := make(chan struct{})
	go func() { io.Copy(os.Stdout, c); close(done) }()
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if i := strings.IndexByte(string(buf[:n]), 0x1d); i >= 0 {
				c.Write(buf[:i])
				break
			}
			c.Write(buf[:n])
			if err != nil {
				break
			}
		}
		c.Close()
	}()
	<-done
	return nil
}

func stty(args ...string) (string, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "kubesilicon:", err)
		os.Exit(1)
	}
}
