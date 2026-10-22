// Package image prepares a raw disk and a cloud-init ISO for a VM.
package image

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Disk creates dest (raw) from src (http(s) URL or local path, qcow2 or raw).
// size > 0 resizes the disk. Downloads are cached in cacheDir, keyed by URL.
func Disk(src, cacheDir, dest string, size int64) error {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		p, err := fetch(src, cacheDir)
		if err != nil {
			return err
		}
		src = p
	}
	if out, err := exec.Command("qemu-img", "convert", "-O", "raw", src, dest).CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img convert: %v: %s", err, out)
	}
	if size > 0 {
		if out, err := exec.Command("qemu-img", "resize", "-f", "raw", dest, fmt.Sprint(size)).CombinedOutput(); err != nil {
			return fmt.Errorf("qemu-img resize: %v: %s", err, out)
		}
	}
	return nil
}

func fetch(url, cacheDir string) (string, error) {
	p := filepath.Join(cacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(url)))[:16])
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp(cacheDir, "dl-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return p, os.Rename(f.Name(), p)
}

// Seed builds the NoCloud ISO (volume "cidata") at iso, using dir as scratch space.
func Seed(dir, iso, hostname, instanceID, userData string) error {
	if userData == "" {
		userData = "#cloud-config\n"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, hostname)
	if err := os.WriteFile(filepath.Join(dir, "user-data"), []byte(userData), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta-data"), []byte(meta), 0o644); err != nil {
		return err
	}
	os.Remove(iso)
	if out, err := exec.Command("hdiutil", "makehybrid", "-iso", "-joliet", "-default-volume-name", "cidata", "-o", iso, dir).CombinedOutput(); err != nil {
		return fmt.Errorf("hdiutil: %v: %s", err, out)
	}
	return nil
}
