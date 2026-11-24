package bunpath

import (
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestShellPath(t *testing.T) {
	assert.Equal(t, shellPath("windows", `C:\Users\circleci\.bun\bin`), "/c/Users/circleci/.bun/bin")
	assert.Equal(t, shellPath("linux", "/home/circleci/.bun/bin"), "/home/circleci/.bun/bin")
}

func TestGlobalBinDir(t *testing.T) {
	t.Setenv("BUN_INSTALL", "/opt/bun")
	dir, err := GlobalBinDir()
	assert.NilError(t, err)
	assert.Equal(t, dir, filepath.Join("/opt/bun", "bin"))

	t.Setenv("BUN_INSTALL", "")
	t.Setenv("HOME", "/home/circleci")
	t.Setenv("USERPROFILE", "/home/circleci")
	dir, err = GlobalBinDir()
	assert.NilError(t, err)
	assert.Equal(t, dir, filepath.Join("/home/circleci", ".bun", "bin"))
}
