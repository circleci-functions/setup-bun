package bundist

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	hc "github.com/circleci-functions/toolkit/httpcl"
)

// DefaultURL is where Bun's official builds are downloaded from.
const DefaultURL = "https://github.com/oven-sh/bun/releases/download"

// Dist is a server holding Bun's release assets: GitHub's release downloads,
// or a mirror laid out the same way, with a bun-v<version>/ directory per
// release holding its zips and SHASUMS256.txt.
type Dist struct {
	url    string
	client *hc.Client
}

// New returns a Dist for the server at url.
func New(ctx context.Context, url string) *Dist {
	url = strings.TrimSuffix(url, "/")
	return &Dist{url: url, client: hc.New(ctx, hc.Config{
		Name:    "bun-dist",
		BaseURL: url,
		Timeout: 15 * time.Minute,
	})}
}

// BinDir returns the directory of an install at root holding bun and bunx.
func BinDir(root string) string {
	return filepath.Join(root, "bin")
}

// Install downloads the build of version for t, checks it against the
// release's SHASUMS256.txt, and unpacks it so that dest/bin holds bun and
// bunx. Anything already at dest, such as an incomplete install, is replaced.
func (d *Dist) Install(ctx context.Context, version string, t Target, dest string) error {
	release := "bun-v" + version
	name := t.ArchiveName()

	want, err := d.checksum(ctx, release, name)
	if err != nil {
		return err
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0750); err != nil {
		return err
	}
	archive, err := os.CreateTemp(parent, name+".*.download")
	if err != nil {
		return err
	}
	defer func() {
		_ = archive.Close()
		_ = os.Remove(archive.Name())
	}()

	if err := d.download(ctx, release, name, archive); err != nil {
		return err
	}
	if err := verify(archive, want); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	staging, err := os.MkdirTemp(parent, filepath.Base(dest)+".*.extract")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	unpacked := filepath.Join(staging, "zip")
	if err := extractZip(archive.Name(), unpacked); err != nil {
		return fmt.Errorf("extract %s: %w", name, err)
	}
	bun := filepath.Join(unpacked, t.DirName(), t.Exe("bun"))
	if _, err := os.Stat(bun); err != nil {
		return fmt.Errorf("%s has no %s/%s", name, t.DirName(), t.Exe("bun"))
	}

	root := filepath.Join(staging, "root")
	bin := BinDir(root)
	if err := os.MkdirAll(bin, 0750); err != nil {
		return err
	}
	if err := os.Rename(bun, filepath.Join(bin, t.Exe("bun"))); err != nil {
		return err
	}
	if err := linkBunx(bin, t); err != nil {
		return err
	}

	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(root, dest)
}

// linkBunx adds bunx beside bun in bin. Bun ships one executable, and runs as
// bunx when invoked under that name, which is how its own installer provides
// it. On Windows, where a symlink needs a privilege jobs may lack, it is a
// hard link, or failing that a copy.
func linkBunx(bin string, t Target) error {
	bunx := filepath.Join(bin, t.Exe("bunx"))
	if t.OS != "windows" {
		return os.Symlink(t.Exe("bun"), bunx)
	}
	bun := filepath.Join(bin, t.Exe("bun"))
	if err := os.Link(bun, bunx); err == nil {
		return nil
	}
	return copyFile(bun, bunx)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is the bun this package just unpacked
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755) //nolint:gosec // must be executable
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// download writes the asset name from release's directory to f, starting
// over on each retry.
func (d *Dist) download(ctx context.Context, release, name string, f *os.File) error {
	err := d.client.Call(ctx, hc.NewRequest(http.MethodGet, "/%s/%s",
		hc.RouteParams(release, name),
		hc.SuccessDecoder(func(r io.Reader) error {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			if err := f.Truncate(0); err != nil {
				return err
			}
			_, err := io.Copy(f, r)
			return err
		}),
		hc.Timeout(10*time.Minute),
	))
	if err != nil {
		return fmt.Errorf("download %s/%s/%s: %w", d.url, release, name, err)
	}
	return nil
}

func (d *Dist) checksum(ctx context.Context, release, name string) (string, error) {
	var sums []byte
	err := d.client.Call(ctx, hc.NewRequest(http.MethodGet, "/%s/SHASUMS256.txt",
		hc.RouteParams(release),
		hc.BytesDecoder(&sums),
		hc.Timeout(time.Minute),
	))
	if err != nil {
		return "", fmt.Errorf("fetch %s/%s/SHASUMS256.txt: %w", d.url, release, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not listed in %s/%s/SHASUMS256.txt: this release has no build for %s",
		name, d.url, release, strings.TrimSuffix(strings.TrimPrefix(name, "bun-"), ".zip"))
}

func verify(f *os.File, want string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch: got sha256 %s, want %s", got, want)
	}
	return nil
}
