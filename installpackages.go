package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/toolkit/sdk"
)

// lockfiles are the lockfiles bun reads, in the order it prefers them: bun.lock
// is the text format Bun 1.2 made the default, bun.lockb the binary one before
// it.
var lockfiles = []string{"bun.lock", "bun.lockb"}

// installPackagesCmd runs a frozen bun install, restoring and saving Bun's
// package cache around it so later jobs start warm.
type installPackagesCmd struct {
	AppDir          string `help:"Directory containing package.json and bun.lock or bun.lockb." default:"." example:"./frontend"`
	OverrideCommand string `help:"Command to run instead of bun install --frozen-lockfile (e.g. 'bun install --frozen-lockfile --production')."`
	Cache           bool   `help:"Restore Bun's package cache before installing and save it afterwards. Set to false to disable caching." default:"true"`
}

func (c *installPackagesCmd) Run(ctx context.Context, client *sdk.Client) error {
	appDir, err := filepath.Abs(c.AppDir)
	if err != nil {
		return fmt.Errorf("resolve app-dir: %w", err)
	}
	info, err := os.Stat(appDir)
	if err != nil {
		return fmt.Errorf("app-dir %q: %w", appDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("app-dir %q is not a directory", appDir)
	}

	if _, err := exec.LookPath("bun"); err != nil {
		return errors.New("bun not found on PATH; run setup-bun without a command in an earlier step to install it")
	}

	args := []string{"bun", "install", "--frozen-lockfile"}
	if c.OverrideCommand != "" {
		if args = strings.Fields(c.OverrideCommand); len(args) == 0 {
			return errors.New("override-command is empty")
		}
	}

	lockfile := findLockfile(appDir)
	if lockfile == "" && c.OverrideCommand == "" {
		return fmt.Errorf("no bun.lock or bun.lockb in %s; a frozen install needs one, so run bun install and commit the lockfile it writes", appDir)
	}

	var key, cacheDir string
	if c.Cache {
		key, cacheDir = restorePackages(ctx, client, appDir, lockfile)
	}

	fmt.Fprintf(os.Stderr, "setup-bun: running %s in %s\n", strings.Join(args, " "), appDir)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args[0] is bun or the caller's own override command
	cmd.Dir = appDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(args, " "), err)
	}

	if key != "" {
		sdk.LogCacheErr("save", client.SaveCache(ctx, key, []string{cacheDir}, sdk.WithStepName("Saving Bun package cache")))
	}
	return nil
}

// findLockfile returns the path of the lockfile in appDir bun will read, or
// "" if there is none.
func findLockfile(appDir string) string {
	for _, name := range lockfiles {
		path := filepath.Join(appDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// restorePackages restores Bun's package cache, and returns the key to save it
// back under and the directory to save. The key is empty, so nothing is
// restored or saved, when bun can't report its cache directory or version, or
// there is no lockfile to key on, as with an --override-command run without
// one. Each of those is a warning rather than an error, so the install still
// runs.
func restorePackages(ctx context.Context, client *sdk.Client, appDir, lockfile string) (key, dir string) {
	if lockfile == "" {
		fmt.Fprintf(os.Stderr, "setup-bun: no bun.lock or bun.lockb found, skipping the package cache\n")
		return "", ""
	}
	dir, err := bunCacheDir(ctx, appDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup-bun: warning: %v; skipping the package cache\n", err)
		return "", ""
	}
	// Worth reading even on a pass: a later step reaches this by asking bun
	// the same question, so a wrong answer here is a cache that never gets
	// used.
	fmt.Fprintf(os.Stderr, "setup-bun: Bun package cache directory %s\n", dir)

	version, err := bunOutput(ctx, appDir, "--version")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup-bun: warning: %v; skipping the package cache\n", err)
		return "", ""
	}
	data, err := os.ReadFile(lockfile) //nolint:gosec // one of a fixed set of lockfile names in the app dir
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup-bun: warning: %v; skipping the package cache\n", err)
		return "", ""
	}

	keys := packagesCacheKeys(version, dir, data)
	before := entryNames(dir)
	_, err = client.RestoreCache(ctx, keys, sdk.WithStepName("Restoring Bun package cache"))
	sdk.LogCacheErr("restore", err)
	// RestoreCache reports success even on a miss, so the only way to tell a
	// hit is to check whether it added to dir.
	if err == nil && addedEntries(dir, before) {
		fmt.Fprintf(os.Stderr, "setup-bun: restored %s from cache\n", dir)
	}
	return keys[0], dir
}

// packagesCacheKeys returns the keys to restore Bun's package cache from, best
// first; the first is also the key to save it under. After the exact key, a
// prefix of it matches the newest cache saved for any lockfile, so a changed
// lockfile downloads only what changed.
//
// The cache directory and Bun's version go before the lockfile hash, so the
// fallback never crosses them: a cache restored anywhere else is never read,
// and Bun's version sets the cache's layout.
func packagesCacheKeys(bunVersion, cacheDir string, lockfile []byte) []string {
	dirHash := sha256.Sum256([]byte(cacheDir))
	prefix := fmt.Sprintf("setup-bun-packages-v1-%s-%x-%s-", sdk.OSArch(), dirHash[:4], bunVersion)
	return []string{fmt.Sprintf("%s%x", prefix, sha256.Sum256(lockfile)), prefix}
}

// bunCacheDir returns the directory bun install caches packages in. bun pm
// cache answers from inside a project, honouring $BUN_INSTALL_CACHE_DIR and
// the project's bunfig.toml, which a hardcoded path would not.
func bunCacheDir(ctx context.Context, appDir string) (string, error) {
	dir, err := bunOutput(ctx, appDir, "pm", "cache")
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("bun pm cache printed nothing")
	}
	return dir, nil
}

// bunOutput runs bun with args in dir and returns its trimmed stdout.
func bunOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("bun %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("bun %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// entryNames returns the names of dir's entries, empty if it cannot be read.
func entryNames(dir string) map[string]bool {
	names := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

// addedEntries reports whether dir now holds an entry that before did not.
func addedEntries(dir string, before map[string]bool) bool {
	for name := range entryNames(dir) {
		if !before[name] {
			return true
		}
	}
	return false
}
