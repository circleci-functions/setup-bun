package bundist

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/toolkit/archive"
)

// extractZip unpacks the zip at src into dest.
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		target, err := within(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// extractZipFile writes f to target. Bun's zips record bun as executable, but
// the bit is set regardless, since without it the install is useless.
func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return archive.WriteFile(target, rc, f.Mode().Perm()|0700) //nolint:gosec // decompression size is bounded by the checksummed archive
}

// within joins name onto dest, rejecting any name that would land outside it.
func within(dest, name string) (string, error) {
	target := filepath.Join(dest, filepath.FromSlash(name))
	if target != filepath.Clean(dest) && !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry %q escapes the destination", name)
	}
	return target, nil
}
