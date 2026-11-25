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

	"github.com/circleci-functions/toolkit/sdk"
	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// TestAcceptance_InstallsOntoTheRealPath is the one test that lets setup-bun
// choose where bun is linked, rather than telling it with --bin-dir. That
// choice depends on the executor, and a sandboxed suite never reaches it.
//
// It changes the machine it runs on, so it runs only where that is
// disposable: a CircleCI job, or a container the caller vouches for with
// $SETUP_BUN_DISPOSABLE (see `task smoke-local`).
func TestAcceptance_InstallsOntoTheRealPath(t *testing.T) {
	if os.Getenv("SETUP_BUN_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}

	s := sdktest.NewSandbox(t, sdktest.WithLiveCache())
	spec := acceptanceVersion()

	// The job's own environment, which is what a step gets: $HOME and PATH
	// decide which directory is picked. $BASH_ENV is redirected because CI
	// writes to the job's own.
	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	run := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		opts := []runner.Opt{
			runner.WithUnsafeOSEnvVars(),
			runner.WithUnsafeAdditionalEnvVars(map[string]string{"BASH_ENV": bashEnv}),
		}
		if dir != "" {
			opts = append(opts, runner.WithCmdDir(dir))
		}
		r := runner.New(t, binaryPath, opts...)
		_, err := r.Run(args...)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "got: %s", out)
		return out
	}

	installArgs := []string{
		"--bun-version", spec,
		"--install-dir", s.InstallDir,
		// Isolates this run's cache entry, so the cold install below cannot
		// restore one an earlier pipeline saved.
		"--cache-nonce", s.CacheKeyNonce,
	}
	out := run(t, "", installArgs...)
	assert.Assert(t, !strings.Contains(out, "from cache"),
		"a per-run nonce must miss the cache, so this install downloads for real; got: %s", out)

	version := reported(t, out, `Bun ([0-9.]+) installed at`)
	if exactVersion.MatchString(spec) {
		assert.Equal(t, version, strings.TrimPrefix(spec, "v"))
	}
	// Worth reading even when this passes: which directory an executor offered
	// is the thing that goes wrong.
	linked := reported(t, out, `linked into (.+)$`)
	t.Logf("setup-bun installed Bun %s and linked it into %s", version, linked)

	// The install dir goes away with the sandbox, so the links must too, or a
	// dangling bun follows this test into the rest of the job.
	removeLinks(t, linked)

	t.Run("ResolvesByNameInALaterStep", func(t *testing.T) {
		for _, name := range []string{"bun", "bunx"} {
			got, err := sdktest.LaterStep(t, name, "--version")
			assert.NilError(t, err, "a later step could not resolve %s: %s", name, got)
			assert.Equal(t, strings.TrimSpace(got), version, name)
		}
	})

	if runtime.GOOS == "windows" {
		t.Run("ResolvesInPowerShellToo", func(t *testing.T) {
			got, err := sdktest.LaterStep(t, "powershell.exe", "-Command", "bun --version; bunx --version")
			assert.NilError(t, err, "PowerShell could not resolve bun and bunx: %s", got)
			assert.Assert(t, strings.Contains(got, version), "PowerShell resolved a different bun: %s", got)
		})
	}

	t.Run("PutsGlobalInstallsOnPathForBash", func(t *testing.T) {
		content, err := os.ReadFile(bashEnv) //nolint:gosec // a path this test chose
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(content), "export PATH="), "got: %s", content)
		assert.Assert(t, strings.Contains(filepath.ToSlash(string(content)), ".bun/bin"), "got: %s", content)
	})

	t.Run("RestoresFromTheCacheOnAFreshExecutor", func(t *testing.T) {
		skip.If(t, !sdk.InCircleCIJob(), "the cache round trip needs a CircleCI job's cache")
		assert.NilError(t, os.RemoveAll(s.InstallDir))
		out := run(t, "", installArgs...)
		assert.Assert(t, strings.Contains(out, "restored Bun "+version+" from cache"),
			"expected the install saved above to be restored; got: %s", out)
		got, err := sdktest.LaterStep(t, "bun", "--version")
		assert.NilError(t, err, got)
		assert.Equal(t, strings.TrimSpace(got), version)
	})
}

// removeLinks takes bun and bunx back off PATH at the end of the test.
func removeLinks(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range []string{"bun", "bunx"} {
			for _, ext := range []string{"", ".exe", ".bat"} {
				_ = os.Remove(filepath.Join(dir, name+ext))
			}
		}
	})
}
