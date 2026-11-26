// Package unixsock listens on and dials unix sockets whose full path exceeds the
// macOS limit (about 104 bytes): it binds by file name from inside the directory.
// ponytail: changes the process cwd for the call, so not safe next to code that
// resolves relative paths concurrently.
package unixsock

import (
	"net"
	"os"
	"path/filepath"
)

// Listen listens on path. The socket file is left behind on Close.
func Listen(path string) (net.Listener, error) {
	var ln net.Listener
	err := in(path, func(name string) (err error) {
		ln, err = net.Listen("unix", name)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Close would unlink the bare name relative to whatever the cwd is by then.
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	return ln, nil
}

// Dial connects to the socket at path.
func Dial(path string) (c net.Conn, err error) {
	err = in(path, func(name string) (err error) {
		c, err = net.Dial("unix", name)
		return err
	})
	return c, err
}

func in(path string, f func(name string) error) error {
	old, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(filepath.Dir(path)); err != nil {
		return err
	}
	defer os.Chdir(old)
	return f(filepath.Base(path))
}
