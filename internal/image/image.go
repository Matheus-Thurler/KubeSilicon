// Package image prepares a raw disk and a cloud-init ISO for a VM.
package image

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Disk creates dest (raw) from src: an http(s) URL, or a local file under localDir (qcow2 or raw).
// sum, when set, is the expected SHA-256 (hex) of a downloaded file. size > 0 resizes the disk.
// Downloads are cached in cacheDir, keyed by URL and sum.
//
// src comes from a cluster object, so it is untrusted: a local path outside localDir, or an image
// that points at another file (qcow2 backing file or external data file), would copy a file of
// the Mac into the guest disk. Both are rejected.
func Disk(src, sum, cacheDir, localDir, dest string, size int64) error {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		p, err := fetch(src, sum, cacheDir)
		if err != nil {
			return err
		}
		src = p
	} else {
		p, err := under(localDir, src)
		if err != nil {
			return err
		}
		src = p
	}
	format, err := probe(src)
	if err != nil {
		return err
	}
	// -f pins the probed format, so convert cannot read the file as something else.
	if out, err := exec.Command("qemu-img", "convert", "-f", format, "-O", "raw", src, dest).CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img convert: %v: %s", err, out)
	}
	if size > 0 {
		if out, err := exec.Command("qemu-img", "resize", "-f", "raw", dest, fmt.Sprint(size)).CombinedOutput(); err != nil {
			return fmt.Errorf("qemu-img resize: %v: %s", err, out)
		}
	}
	return nil
}

// under resolves symlinks and returns path only if it is inside dir.
func under(dir, path string) (string, error) {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("local disk sources are disabled: %v", err)
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.HasPrefix(p, d+string(filepath.Separator)) {
		return "", fmt.Errorf("local disk source must be a file under %s", dir)
	}
	return p, nil
}

// probe returns the image format, accepting only self-contained raw and qcow2 images.
func probe(path string) (string, error) {
	out, err := exec.Command("qemu-img", "info", "--output=json", path).Output()
	if err != nil {
		return "", fmt.Errorf("qemu-img info: %v", err)
	}
	var info struct {
		Format   string `json:"format"`
		Backing  string `json:"backing-filename"`
		Specific struct {
			Data struct {
				DataFile string `json:"data-file"`
			} `json:"data"`
		} `json:"format-specific"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("qemu-img info: %v", err)
	}
	if info.Format != "raw" && info.Format != "qcow2" {
		return "", fmt.Errorf("disk image format %q is not supported (raw or qcow2)", info.Format)
	}
	if info.Backing != "" || info.Specific.Data.DataFile != "" {
		return "", fmt.Errorf("disk image references another file (backing or data file); only self-contained images are accepted")
	}
	return info.Format, nil
}

// ponytail: one overall deadline, so a stalled download fails instead of blocking the worker
// forever. A very slow link needs a bigger value or a per-read idle timeout.
var client = &http.Client{Timeout: time.Hour}

func fetch(url, sum, cacheDir string) (string, error) {
	p := filepath.Join(cacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(url+sum)))[:16])
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	resp, err := client.Get(url)
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
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); sum != "" && !strings.EqualFold(got, sum) {
		return "", fmt.Errorf("GET %s: sha256 is %s, spec.disk.sha256 says %s", url, got, sum)
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
