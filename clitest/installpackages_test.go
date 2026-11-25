package clitest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/testing/runner"
)

// stubBun is a bun that reports version 1.4.2 and $STUB_BUN_CACHE as its
// cache, and for install records its working directory and arguments to
// $STUB_BUN_LOG and adds a package to the cache, as a real download would.
const stubBun = `#!/bin/sh
case "$1" in
--version) echo 1.4.2 ;;
pm) echo "$STUB_BUN_CACHE" ;;
install)
  echo "$PWD $*" >> "$STUB_BUN_LOG"
  [ -n "$STUB_BUN_FAIL" ] && { echo "error: lockfile had changes, but lockfile is frozen" >&2; exit 1; }
  mkdir -p "$STUB_BUN_CACHE/is-odd@3.0.1"
  ;;
esac
`

type bunStub struct {
	env   map[string]string
	cache string
	log   string
}

func newBunStub(t *testing.T) bunStub {
	t.Helper()
	dir := t.TempDir()
	b := bunStub{cache: filepath.Join(dir, "cache"), log: filepath.Join(dir, "log")}
	b.env = map[string]string{
		"PATH":           stubPATH(t, map[string]string{"bun": stubBun}),
		"STUB_BUN_CACHE": b.cache,
		"STUB_BUN_LOG":   b.log,
	}
	return b
}

func (b bunStub) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(b.log)
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// appDir returns a project directory holding a package.json and lockfile.
func appDir(t *testing.T, lockfile string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies": {"is-odd": "^3.0.1"}}`)
	if lockfile != "" {
		writeFile(t, dir, lockfile, "lockfile")
	}
	return dir
}

func TestInstallPackages_RunsAFrozenInstall(t *testing.T) {
	skipOnWindows(t)
	_, run := sandbox(t)
	bun := newBunStub(t)
	app := appDir(t, "bun.lock")

	out, err := run(t, bun.env, "install-packages", "--app-dir", app)
	assert.NilError(t, err, "got: %s", out)
	assert.DeepEqual(t, bun.calls(t), []string{app + " install --frozen-lockfile"})
}

func TestInstallPackages_DefaultsToTheWorkingDirectory(t *testing.T) {
	skipOnWindows(t)
	app := appDir(t, "bun.lockb")
	_, run := sandbox(t, runner.WithCmdDir(app))
	bun := newBunStub(t)

	out, err := run(t, bun.env, "install-packages")
	assert.NilError(t, err, "got: %s", out)
	// The process's working directory has its symlinks resolved, as macOS's
	// /var is.
	real, err := filepath.EvalSymlinks(app)
	assert.NilError(t, err)
	assert.DeepEqual(t, bun.calls(t), []string{real + " install --frozen-lockfile"})
}

func TestInstallPackages_CachesThePackages(t *testing.T) {
	skipOnWindows(t)
	s, run := sandbox(t)
	bun := newBunStub(t)
	app := appDir(t, "bun.lock")

	out, err := run(t, bun.env, "install-packages", "--app-dir", app)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "Bun package cache directory "+bun.cache), "got: %s", out)
	assert.Assert(t, !strings.Contains(out, "from cache"), "got: %s", out)
	keys := s.Cache.SavedKeys()
	assert.Equal(t, len(keys), 1)
	assert.Assert(t, strings.HasPrefix(keys[0], "setup-bun-packages-v1-"), keys[0])

	// A fresh executor: the cache is gone, so it has to come back.
	assert.NilError(t, os.RemoveAll(bun.cache))
	out, err = run(t, bun.env, "install-packages", "--app-dir", app)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "restored "+bun.cache+" from cache"), "got: %s", out)
}

func TestInstallPackages_AChangedLockfileRestoresTheLastCache(t *testing.T) {
	skipOnWindows(t)
	s, run := sandbox(t)
	bun := newBunStub(t)
	app := appDir(t, "bun.lock")

	out, err := run(t, bun.env, "install-packages", "--app-dir", app)
	assert.NilError(t, err, "got: %s", out)

	assert.NilError(t, os.RemoveAll(bun.cache))
	writeFile(t, app, "bun.lock", "changed")
	out, err = run(t, bun.env, "install-packages", "--app-dir", app)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "from cache"), "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 2)
}

func TestInstallPackages_CacheFalse(t *testing.T) {
	skipOnWindows(t)
	s, run := sandbox(t)
	bun := newBunStub(t)

	out, err := run(t, bun.env, "install-packages", "--app-dir", appDir(t, "bun.lock"), "--cache=false")
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 0)
	assert.Assert(t, !strings.Contains(out, "package cache directory"), "got: %s", out)
}

func TestInstallPackages_OverrideCommand(t *testing.T) {
	skipOnWindows(t)
	_, run := sandbox(t)
	bun := newBunStub(t)
	app := appDir(t, "bun.lock")

	out, err := run(t, bun.env, "install-packages", "--app-dir", app,
		"--override-command", "bun install --frozen-lockfile --production")
	assert.NilError(t, err, "got: %s", out)
	assert.DeepEqual(t, bun.calls(t), []string{app + " install --frozen-lockfile --production"})
}

func TestInstallPackages_OverrideCommandNeedsNoLockfile(t *testing.T) {
	skipOnWindows(t)
	s, run := sandbox(t)
	bun := newBunStub(t)
	app := appDir(t, "")

	out, err := run(t, bun.env, "install-packages", "--app-dir", app, "--override-command", "bun install")
	assert.NilError(t, err, "got: %s", out)
	assert.DeepEqual(t, bun.calls(t), []string{app + " install"})
	assert.Assert(t, strings.Contains(out, "skipping the package cache"), "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 0)
}

func TestInstallPackages_NoLockfileFails(t *testing.T) {
	skipOnWindows(t)
	_, run := sandbox(t)
	bun := newBunStub(t)

	out, err := run(t, bun.env, "install-packages", "--app-dir", appDir(t, ""))
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "no bun.lock or bun.lockb"), "got: %s", out)
	assert.Equal(t, len(bun.calls(t)), 0)
}

func TestInstallPackages_FailedInstallFailsAndSavesNothing(t *testing.T) {
	skipOnWindows(t)
	s, run := sandbox(t)
	bun := newBunStub(t)
	bun.env["STUB_BUN_FAIL"] = "1"

	out, err := run(t, bun.env, "install-packages", "--app-dir", appDir(t, "bun.lock"))
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "lockfile is frozen"), "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 0)
}

func TestInstallPackages_NoBunOnPathFails(t *testing.T) {
	skipOnWindows(t)
	_, run := sandbox(t)

	out, err := run(t, map[string]string{"PATH": noBunPATH()}, "install-packages", "--app-dir", appDir(t, "bun.lock"))
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "run setup-bun without a command"), "got: %s", out)
}

func TestInstallPackages_AppDirMustBeADirectory(t *testing.T) {
	_, run := sandbox(t)
	file := writeFile(t, t.TempDir(), "package.json", "{}")

	out, err := run(t, nil, "install-packages", "--app-dir", file)
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "is not a directory"), "got: %s", out)

	out, err = run(t, nil, "install-packages", "--app-dir", filepath.Join(t.TempDir(), "missing"))
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "no such file"), "got: %s", out)
}
