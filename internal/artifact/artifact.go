// Package artifact moves job inputs and outputs over plain HTTP (GET and PUT), so presigned
// MinIO/S3 URLs work and the job never sees credentials.
// ponytail: no S3 client or Secret lookup; the pipeline hands over presigned URLs. Add Secret-based S3 if needed.
package artifact

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Path joins rel under dir and rejects anything that escapes it.
func Path(dir, rel string) (string, error) {
	p := filepath.Join(dir, rel)
	if rel == "" || filepath.IsAbs(rel) || (p != dir && !strings.HasPrefix(p, dir+string(filepath.Separator))) {
		return "", fmt.Errorf("path %q must be relative and stay inside the work dir", rel)
	}
	return p, nil
}

func Download(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return fmt.Errorf("GET %s: %s", redact(url), r.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r.Body)
	return err
}

func Upload(src, url string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url, f)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return fmt.Errorf("PUT %s: %s", redact(url), r.Status)
	}
	return nil
}

// redact drops the query string: presigned URLs carry the signature there.
func redact(u string) string { s, _, _ := strings.Cut(u, "?"); return s }
