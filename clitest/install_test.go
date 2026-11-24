package clitest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"

	"github.com/circleci-functions/setup-bun/internal/testing/fakebun"
)

// installArgs points setup-bun at srv and at s's directories.
func installArgs(s *sdktest.Sandbox, srv *fakebun.Server, extra ...string) []string {
	return append([]string{
		"--mirror", srv.URL,
		"--registry", srv.URL,
		"--install-dir", s.InstallDir,
		"--bin-dir", s.BinDir,
	}, extra...)
}

func releases(t *testing.T) *fakebun.Server {
	t.Helper()
	return fakebun.New(t,
		fakebun.Release{Version: "1.3.14"},
		fakebun.Release{Version: "1.4.2"},
		fakebun.Release{Version: "1.4.5"},
	)
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	skip.If(t, runtime.GOOS == "windows", "the fake bun is a shell script")
}

// runLinked runs name from the directory setup-bun linked into.
func runLinked(t *testing.T, s *sdktest.Sandbox, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(filepath.Join(s.BinDir, name), args...).Output() //nolint:gosec // the binary the test just linked
	assert.NilError(t, err)
	return strings.TrimSpace(string(out))
}

func TestInstall_ResolvesDownloadsAndLinks(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "1.4")...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, `resolved "1.4" to Bun 1.4.5`), "got: %s", out)
	assert.Assert(t, strings.Contains(out, "linked into "+s.BinDir), "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.5")
	assert.Equal(t, runLinked(t, s, "bunx", "--version"), "1.4.5")
	assert.Equal(t, srv.Downloads(), 1)

	bashEnv, err := os.ReadFile(s.BashEnv)
	assert.NilError(t, err)
	assert.Equal(t, string(bashEnv), "export PATH="+filepath.Join(s.Home, ".bun", "bin")+":$PATH\n")
}

func TestInstall_BareStepRunsInstall(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	// The install subcommand is the default, so a step with no command runs it
	// and its flags are accepted at the top level.
	out, err := run(t, nil, append([]string{"install"}, installArgs(s, srv, "--bun-version", "1.4.2")...)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.2")
}

func TestInstall_ExactVersionSkipsTheRegistry(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "v1.4.2")...)
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.2")
	assert.Equal(t, srv.RegistryRequests(), 0)
}

func TestInstall_Latest(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "latest")...)
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.5")
}

func TestInstall_RestoresFromTheCache(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)
	args := installArgs(s, srv, "--bun-version", "1.4.2")

	out, err := run(t, nil, args...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, !strings.Contains(out, "from cache"), "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 1)

	// A fresh executor: the install is gone, so it has to come from the cache.
	assert.NilError(t, os.RemoveAll(s.InstallDir))
	out, err = run(t, nil, args...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "restored Bun 1.4.2 from cache"), "got: %s", out)
	assert.Equal(t, srv.Downloads(), 1)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.2")
}

func TestInstall_ReusesAnInstallAlreadyThere(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)
	args := installArgs(s, srv, "--bun-version", "1.4.2", "--cache=false")

	for range 2 {
		out, err := run(t, nil, args...)
		assert.NilError(t, err, "got: %s", out)
	}
	assert.Equal(t, srv.Downloads(), 1)
}

func TestInstall_CacheFalseSavesNothing(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "1.4.2", "--cache=false")...)
	assert.NilError(t, err, "got: %s", out)
	assert.Equal(t, len(s.Cache.SavedKeys()), 0)
}

func TestInstall_VersionFile(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)
	file := writeFile(t, t.TempDir(), "package.json", `{"packageManager": "bun@1.3.14"}`)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version-file", file)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, `read Bun version "1.3.14" from `+file), "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.3.14")
}

func TestInstall_VersionWinsOverVersionFile(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)
	file := writeFile(t, t.TempDir(), ".bun-version", "1.3.14\n")

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "1.4.2", "--bun-version-file", file)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "using --bun-version"), "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.2")
}

func TestInstall_ReadsTheWorkingDirectory(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	wd := t.TempDir()
	writeFile(t, wd, ".bun-version", "1.3\n")
	s, run := sandbox(t, runner.WithCmdDir(wd))

	out, err := run(t, nil, installArgs(s, srv)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, `read Bun version "1.3" from `), "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.3.14")
}

func TestInstall_UsesTheBunOnPathWhenNoneIsAskedFor(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)
	path := stubPATH(t, map[string]string{"bun": "#!/bin/sh\necho 1.2.0\n"})

	out, err := run(t, map[string]string{"PATH": path}, installArgs(s, srv)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "using the Bun 1.2.0 already on PATH"), "got: %s", out)
	assert.Equal(t, len(srv.Requests()), 0)
}

func TestInstall_InstallsLatestWithNothingToGoOn(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, map[string]string{"PATH": noBunPATH()}, installArgs(s, srv)...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "using latest"), "got: %s", out)
	assert.Equal(t, runLinked(t, s, "bun", "--version"), "1.4.5")
}

func TestInstall_ChecksumMismatchFails(t *testing.T) {
	srv := releases(t)
	srv.Corrupt()
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "1.4.2")...)
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, "checksum mismatch"), "got: %s", out)
	_, statErr := os.Stat(filepath.Join(s.BinDir, "bun"))
	assert.Assert(t, os.IsNotExist(statErr), "nothing may be linked after a failed install")
}

func TestInstall_UnknownVersionFails(t *testing.T) {
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, nil, installArgs(s, srv, "--bun-version", "2")...)
	assert.ErrorContains(t, err, "exit status")
	assert.Assert(t, strings.Contains(out, `no Bun release matches "2"`), "got: %s", out)
}

func TestInstall_MirrorFromEnv(t *testing.T) {
	skipOnWindows(t)
	srv := releases(t)
	s, run := sandbox(t)

	out, err := run(t, map[string]string{"BUN_MIRROR": srv.URL},
		"--bun-version", "1.4.2", "--install-dir", s.InstallDir, "--bin-dir", s.BinDir)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "from "+srv.URL), "got: %s", out)
}
