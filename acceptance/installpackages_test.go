//go:build acceptance

package setupbun_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"

	"github.com/circleci-functions/toolkit/sdk"
	"github.com/circleci-functions/toolkit/sdk/sdktest"
)

// TestAcceptance_InstallPackages installs Bun, then runs a real frozen bun
// install against the registry.
func TestAcceptance_InstallPackages(t *testing.T) {
	skip.If(t, runtime.GOOS == "windows", "the smoke job covers Windows, where the links are shims")
	s, run := sandbox(t)
	env := installBun(t, s, run)

	app := copyFixture(t)
	// The in-memory cache copies files one by one, and Bun's cache holds
	// symlinks it can't copy; the live round trip is below.
	out, err := run(t, env, "install-packages", "--app-dir", app, "--cache=false")
	assert.NilError(t, err, "got: %s", out)
	assertInstalled(t, app)

	t.Run("FailsOnAStaleLockfile", func(t *testing.T) {
		stale := copyFixture(t)
		assert.NilError(t, os.WriteFile(filepath.Join(stale, "package.json"),
			[]byte(`{"dependencies": {"is-odd": "^3.0.1", "is-even": "^1.0.0"}}`), 0600))
		out, err := run(t, env, "install-packages", "--app-dir", stale, "--cache=false")
		assert.Assert(t, err != nil, "a frozen install must fail when package.json and bun.lock disagree; got: %s", out)
	})
}

// TestAcceptance_InstallPackages_RestoresFromTheLiveCache saves Bun's package
// cache to the job's real cache, then on a fresh executor restores it.
//
// Cache keys are write-once and come from the lockfile, so the project is
// named for this run: its lockfile, and so its key, is one no earlier pipeline
// saved.
func TestAcceptance_InstallPackages_RestoresFromTheLiveCache(t *testing.T) {
	skip.If(t, runtime.GOOS == "windows", "the smoke job covers Windows, where the links are shims")
	skip.If(t, !sdk.InCircleCIJob(), "the cache round trip needs a CircleCI job's cache")
	s := sdktest.NewSandbox(t, sdktest.WithLiveCache())
	run := newRun(s)
	env := installBun(t, s, run)

	app := t.TempDir()
	manifest := `{"name": "smoke-` + strings.ToLower(s.CacheKeyNonce) + `", "dependencies": {"is-odd": "^3.0.1"}}`
	assert.NilError(t, os.WriteFile(filepath.Join(app, "package.json"), []byte(manifest), 0600))
	bun := exec.CommandContext(t.Context(), filepath.Join(s.BinDir, "bun"), "install", "--lockfile-only")
	bun.Dir = app
	bun.Env = s.CmdEnv(os.Environ())
	lockOut, err := bun.CombinedOutput()
	assert.NilError(t, err, "bun install --lockfile-only: %s", lockOut)

	args := []string{"install-packages", "--app-dir", app}
	out, err := run(t, env, args...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, !strings.Contains(out, "from cache"), "the first install has nothing to restore; got: %s", out)
	assertInstalled(t, app)
	cacheDir := reported(t, out, `Bun package cache directory (.+)$`)

	// A fresh executor: the package cache and node_modules are gone.
	assert.NilError(t, os.RemoveAll(cacheDir))
	assert.NilError(t, os.RemoveAll(filepath.Join(app, "node_modules")))

	out, err = run(t, env, args...)
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "restored "+cacheDir+" from cache"), "got: %s", out)
	assertInstalled(t, app)
}

// installBun installs Bun into s, and returns the environment that puts it
// first on PATH.
func installBun(t *testing.T, s *sdktest.Sandbox, run runFunc) map[string]string {
	t.Helper()
	out, err := run(t, nil, "--bun-version", acceptanceVersion(), "--install-dir", s.InstallDir, "--bin-dir", s.BinDir)
	assert.NilError(t, err, "got: %s", out)
	return map[string]string{"PATH": s.BinDir + string(os.PathListSeparator) + os.Getenv("PATH")}
}

func assertInstalled(t *testing.T, app string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(app, "node_modules", "is-odd", "package.json"))
	assert.NilError(t, err, "expected node_modules/is-odd after install")
}
