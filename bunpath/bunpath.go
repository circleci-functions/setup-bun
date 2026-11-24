// Package bunpath makes an unpacked Bun install usable by name for the rest of
// a CircleCI job.
package bunpath

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/circleci-functions/toolkit/bashenv"
	"github.com/circleci-functions/toolkit/pathlink"
)

// Options configures Link.
type Options struct {
	// BinDir is the install's directory holding bun and bunx.
	BinDir string
	// Version is what bun --version must report, e.g. "1.4.2".
	Version string
	// LinkDir links into exactly this directory instead of discovering one on
	// PATH.
	LinkDir string
	// Path is the search path to use. Empty means this process's PATH.
	Path string
}

// Link makes bun and bunx resolvable by name from a directory already on PATH,
// and returns that directory.
func Link(opts Options) (string, error) {
	res, err := pathlink.Ensure(pathlink.Options{
		Names:     []string{"bun", "bunx"},
		FromDir:   opts.BinDir,
		Preferred: pathlink.UserBinDirs(),
		Dir:       opts.LinkDir,
		Path:      opts.Path,
		Verify:    Verify(opts.BinDir, opts.Version),
	})
	if err != nil {
		return "", err
	}
	return res.Dir, nil
}

// Verify accepts only the bun from binDir reporting version, so a preinstalled
// bun that happens to answer the same does not pass for it. On Windows the
// links are shims that call bun.exe by absolute path, so there only the version
// can identify them.
func Verify(binDir, version string) func(string) error {
	want := filepath.Join(binDir, "bun")
	return func(bin string) error {
		got, err := ReportedVersion(bin)
		if err != nil {
			return err
		}
		if got != version {
			return fmt.Errorf("reports %s rather than %s", got, version)
		}
		if runtime.GOOS != "windows" && !sameFile(bin, want) {
			return fmt.Errorf("is not %s", want)
		}
		return nil
	}
}

// ReportedVersion runs bin --version and returns what it prints.
func ReportedVersion(bin string) (string, error) {
	out, err := pathlink.Command(bin, "--version").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("failed to run: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("failed to run: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GlobalBinDir returns where bun add --global puts executables:
// $BUN_INSTALL/bin, which defaults to ~/.bun/bin.
func GlobalBinDir() (string, error) {
	if dir := os.Getenv("BUN_INSTALL"); dir != "" {
		return filepath.Join(dir, "bin"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bun", "bin"), nil
}

// AddGlobalBinDir puts GlobalBinDir on PATH for later steps through
// $BASH_ENV, so executables from bun add --global resolve by name in bash. It
// returns the directory.
func AddGlobalBinDir() (string, error) {
	dir, err := GlobalBinDir()
	if err != nil {
		return "", err
	}
	return dir, bashenv.AddToPath(shellPath(runtime.GOOS, dir))
}

// shellPath returns dir as bash reads it in a PATH entry. On Windows that is
// Git Bash's form, /c/Users/…, since C: would split at the colon.
func shellPath(goos, dir string) string {
	if goos != "windows" {
		return dir
	}
	dir = strings.ReplaceAll(dir, `\`, "/")
	if len(dir) >= 2 && dir[1] == ':' {
		dir = "/" + strings.ToLower(dir[:1]) + dir[2:]
	}
	return dir
}

func sameFile(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
