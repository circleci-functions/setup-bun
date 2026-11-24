package bundist_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"

	"github.com/circleci-functions/setup-bun/bundist"
	"github.com/circleci-functions/setup-bun/internal/testing/fakebun"
)

var linuxX64 = bundist.Target{OS: "linux", Arch: "x64"}

func TestInstall(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.4.2"})
	dest := filepath.Join(t.TempDir(), "1.4.2-linux-x64")

	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "1.4.2", linuxX64, dest)
	assert.NilError(t, err)

	bin := bundist.BinDir(dest)
	info, err := os.Stat(filepath.Join(bin, "bun"))
	assert.NilError(t, err)
	assert.Assert(t, info.Mode().Perm()&0100 != 0, "bun must be executable, got %v", info.Mode())
	link, err := os.Readlink(filepath.Join(bin, "bunx"))
	assert.NilError(t, err)
	assert.Equal(t, link, "bun")

	// Nothing is left beside the install.
	entries, err := os.ReadDir(filepath.Dir(dest))
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)

	skip.If(t, runtime.GOOS == "windows", "the fake bun is a shell script")
	out, err := exec.Command(filepath.Join(bin, "bunx"), "--version").Output()
	assert.NilError(t, err)
	assert.Equal(t, strings.TrimSpace(string(out)), "1.4.2")
}

func TestInstall_ReplacesAnIncompleteInstall(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.4.2"})
	dest := filepath.Join(t.TempDir(), "1.4.2-linux-x64")
	assert.NilError(t, os.MkdirAll(filepath.Join(dest, "stale"), 0750))

	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "1.4.2", linuxX64, dest)
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(dest, "stale"))
	assert.Assert(t, os.IsNotExist(err))
}

func TestInstall_WindowsLayout(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.4.2"})
	dest := filepath.Join(t.TempDir(), "1.4.2-windows-x64")
	windows := bundist.Target{OS: "windows", Arch: "x64"}

	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "1.4.2", windows, dest)
	assert.NilError(t, err)
	for _, name := range []string{"bun.exe", "bunx.exe"} {
		data, err := os.ReadFile(filepath.Join(bundist.BinDir(dest), name))
		assert.NilError(t, err, name)
		assert.Equal(t, string(data), "1.4.2")
	}
}

func TestInstall_ChecksumMismatch(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.4.2"})
	srv.Corrupt()
	dest := filepath.Join(t.TempDir(), "1.4.2-linux-x64")

	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "1.4.2", linuxX64, dest)
	assert.ErrorContains(t, err, "checksum mismatch")
	_, err = os.Stat(dest)
	assert.Assert(t, os.IsNotExist(err), "a bad download must leave no install")
}

func TestInstall_NoBuildForTarget(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.0.0", Targets: []string{"linux-x64"}})

	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "1.0.0",
		bundist.Target{OS: "linux", Arch: "x64", Musl: true}, filepath.Join(t.TempDir(), "x"))
	assert.ErrorContains(t, err, "this release has no build for linux-x64-musl")
	assert.Equal(t, srv.Downloads(), 0)
}

func TestInstall_UnknownVersion(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.4.2"})
	err := bundist.New(context.Background(), srv.URL).Install(context.Background(), "9.9.9", linuxX64, filepath.Join(t.TempDir(), "x"))
	assert.ErrorContains(t, err, "bun-v9.9.9/SHASUMS256.txt")
}

func TestListVersions(t *testing.T) {
	srv := fakebun.New(t, fakebun.Release{Version: "1.3.0"}, fakebun.Release{Version: "1.4.2"})

	got, err := bundist.ListVersions(context.Background(), srv.URL+"/")
	assert.NilError(t, err)
	slices.Sort(got.All)
	assert.DeepEqual(t, got.All, []string{"1.3.0", "1.4.2"})
	assert.Equal(t, got.Latest, "1.4.2")
}

func TestListVersions_Empty(t *testing.T) {
	srv := fakebun.New(t)
	_, err := bundist.ListVersions(context.Background(), srv.URL)
	assert.ErrorContains(t, err, "lists no versions")
}
