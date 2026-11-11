package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/Matheus-Thurler/kubesilicon/internal/artifact"
	"github.com/Matheus-Thurler/kubesilicon/internal/sandbox"
)

type fileRef struct {
	URL  string `json:"url"`
	Path string `json:"path"`
}

// jobSpec is written to <dir>/spec.json by the daemon, so the runner needs no cluster access.
type jobSpec struct {
	Python  string    `json:"python"`
	Deps    []string  `json:"dependencies"`
	Script  string    `json:"script"`
	Command []string  `json:"command"`
	Timeout int       `json:"timeoutSeconds"`
	Inputs  []fileRef `json:"inputs"`
	Outputs []fileRef `json:"outputs"`
}

var (
	pyVersion   = regexp.MustCompile(`^3\.\d+(\.\d+)?$`)
	requirement = regexp.MustCompile(`^[A-Za-z0-9]`)
)

// validate rejects values that uv, which runs outside the sandbox, would treat as something
// other than a version or a requirement: a python path is executed, a "-flag" changes the install.
func (s jobSpec) validate() error {
	if !pyVersion.MatchString(s.Python) {
		return fmt.Errorf("spec.python %q must be a version like 3.12", s.Python)
	}
	for _, d := range s.Deps {
		if !requirement.MatchString(d) {
			return fmt.Errorf("spec.dependencies: %q must start with a package name", d)
		}
	}
	return nil
}

// runjob is the per-job process ("kubesilicond runjob DIR"). Like launch, it survives the daemon.
// It always ends by writing <dir>/exit as "<code> <message>"; the daemon turns that into status.
func runjob(dir string) {
	code, msg := execJob(dir)
	os.WriteFile(filepath.Join(dir, "exit"), []byte(fmt.Sprintf("%d %s", code, msg)), 0o644)
}

func execJob(dir string) (int, string) {
	var s jobSpec
	if b, err := os.ReadFile(filepath.Join(dir, "spec.json")); err != nil || json.Unmarshal(b, &s) != nil {
		return 1, "cannot read spec.json"
	}
	if err := s.validate(); err != nil {
		return 1, err.Error()
	}
	logf, err := os.OpenFile(filepath.Join(dir, "job.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 1, err.Error()
	}
	defer logf.Close()
	say := func(f string, a ...any) { fmt.Fprintf(logf, "[kubesilicon] "+f+"\n", a...) }

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.Timeout)*time.Second)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	signal.Ignore(syscall.SIGHUP)
	terminated := false
	go func() { <-sig; terminated = true; cancel() }()

	work := filepath.Join(dir, "work")
	os.MkdirAll(filepath.Join(work, "tmp"), 0o755)
	for _, in := range s.Inputs {
		p, err := artifact.Path(work, in.Path)
		if err == nil {
			say("input %s", in.Path)
			err = artifact.Download(in.URL, p)
		}
		if err != nil {
			return 1, "input: " + err.Error()
		}
	}

	venv := filepath.Join(dir, "venv")
	uv := tool("uv")
	steps := [][]string{{uv, "venv", "--quiet", "--python", s.Python, venv}}
	if len(s.Deps) > 0 { // binary wheels only: installing must not run build scripts outside the sandbox
		steps = append(steps, append([]string{uv, "pip", "install", "--quiet", "--python", filepath.Join(venv, "bin", "python"), "--only-binary", ":all:"}, s.Deps...))
	}
	for _, a := range steps {
		say("%v", a[1:3])
		c := exec.CommandContext(ctx, a[0], a[1:]...)
		c.Stdout, c.Stderr = logf, logf
		if err := c.Run(); err != nil {
			return end(ctx, terminated, 1, "environment setup failed: "+err.Error())
		}
	}

	argv := s.Command
	if s.Script != "" {
		if err := os.WriteFile(filepath.Join(work, "script.py"), []byte(s.Script), 0o644); err != nil {
			return 1, err.Error()
		}
		argv = []string{"python", "script.py"}
	}
	if len(argv) == 0 {
		return 1, "spec needs script or command"
	}
	if argv[0] == "python" {
		argv = append([]string{filepath.Join(venv, "bin", "python")}, argv[1:]...)
	}

	home, _ := os.UserHomeDir()
	ro := []string{venv}
	if py, err := filepath.EvalSymlinks(filepath.Join(venv, "bin", "python")); err == nil {
		ro = append(ro, filepath.Dir(filepath.Dir(py))) // the interpreter's install prefix
	}
	prof := filepath.Join(dir, "profile.sb")
	if err := os.WriteFile(prof, []byte(sandbox.Profile(home, []string{work}, ro)), 0o600); err != nil {
		return 1, err.Error()
	}
	cmd := exec.Command("/usr/bin/nice", append([]string{"-n", "10"}, sandbox.Argv(prof, argv)...)...)
	cmd.Dir = work
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Minimal environment: nothing from the daemon (tokens, keys) reaches user code.
	cmd.Env = []string{
		"PATH=" + filepath.Join(venv, "bin") + ":/usr/bin:/bin", "HOME=" + work, "TMPDIR=" + filepath.Join(work, "tmp"),
		"VIRTUAL_ENV=" + venv, "PYTHONDONTWRITEBYTECODE=1", "PYTORCH_ENABLE_MPS_FALLBACK=1", "PYTHONUNBUFFERED=1",
	}
	say("run %v", argv)
	if err := cmd.Start(); err != nil {
		return 1, err.Error()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-done:
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		werr = <-done
	}
	if ctx.Err() != nil {
		return end(ctx, terminated, 1, "")
	}
	if werr != nil {
		if ee, ok := werr.(*exec.ExitError); ok {
			return ee.ExitCode(), fmt.Sprintf("command exited with %d", ee.ExitCode())
		}
		return 1, werr.Error()
	}
	for _, o := range s.Outputs {
		p, err := artifact.Path(work, o.Path)
		if err == nil {
			say("output %s", o.Path)
			err = artifact.Upload(p, o.URL)
		}
		if err != nil {
			return 1, "output: " + err.Error()
		}
	}
	return 0, "done"
}

// end maps a cancelled context to the right exit: SIGTERM from the daemon, or the timeout.
func end(ctx context.Context, terminated bool, code int, msg string) (int, string) {
	switch {
	case terminated:
		return 143, "terminated"
	case ctx.Err() == context.DeadlineExceeded:
		return 124, "timed out"
	}
	return code, msg
}

// tool finds a binary even when the LaunchAgent PATH is minimal.
func tool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		if p := filepath.Join(d, name); fileExists(p) {
			return p
		}
	}
	log.Printf("%s not found", name)
	return name
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
