//go:build acceptance

package setupbun_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"

	"github.com/circleci-functions/toolkit/pathlink"
)

// TestAcceptance_Installs downloads a real Bun build for this machine, checks
// it against the release's checksums, and runs what it linked.
func TestAcceptance_Installs(t *testing.T) {
	skip.If(t, runtime.GOOS == "windows", "the smoke job covers Windows, where the links are shims")
	s, run := sandbox(t)
	spec := acceptanceVersion()

	out, err := run(t, nil, "--bun-version", spec, "--install-dir", s.InstallDir, "--bin-dir", s.BinDir)
	assert.NilError(t, err, "got: %s", out)
	version := reported(t, out, `Bun ([0-9.]+) installed at`)
	if exactVersion.MatchString(spec) {
		assert.Equal(t, version, strings.TrimPrefix(spec, "v"))
	}

	for _, name := range []string{"bun", "bunx"} {
		got, err := pathlink.Command(filepath.Join(s.BinDir, name), "--version").Output()
		assert.NilError(t, err, name)
		assert.Equal(t, strings.TrimSpace(string(got)), version, name)
	}
}

// TestAcceptance_ResolvesAPartialVersion lists Bun's versions from the real
// registry.
func TestAcceptance_ResolvesAPartialVersion(t *testing.T) {
	skip.If(t, runtime.GOOS == "windows", "the smoke job covers Windows, where the links are shims")
	s, run := sandbox(t)

	out, err := run(t, nil, "--bun-version", "1.3", "--install-dir", s.InstallDir, "--bin-dir", s.BinDir)
	assert.NilError(t, err, "got: %s", out)
	version := reported(t, out, `resolved "1\.3" to Bun ([0-9.]+)`)
	assert.Assert(t, strings.HasPrefix(version, "1.3."), "got %s", version)

	got, err := pathlink.Command(filepath.Join(s.BinDir, "bun"), "--version").Output()
	assert.NilError(t, err)
	assert.Equal(t, strings.TrimSpace(string(got)), version)
}

// TestAcceptance_UnknownVersionFails asks for a release that doesn't exist.
func TestAcceptance_UnknownVersionFails(t *testing.T) {
	s, run := sandbox(t)
	out, err := run(t, nil, "--bun-version", "0.0.1", "--install-dir", s.InstallDir, "--bin-dir", s.BinDir)
	assert.Assert(t, err != nil, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "bun-v0.0.1/SHASUMS256.txt"), "got: %s", out)
	_, statErr := os.Stat(filepath.Join(s.BinDir, "bun"))
	assert.Assert(t, os.IsNotExist(statErr))
}
