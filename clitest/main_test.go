// Package clitest_test exercises the setup-bun CLI contract end to end: flag
// parsing, error paths, and how the commands drive the cache SDK. It is
// hermetic, with no network and no real Bun: installs come from a fake release
// server and registry, and install-packages runs a stub bun. Real downloads,
// real installs and real-infra cache round-trips are covered by acceptance/.
package clitest_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
)

var binaryPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	p := compiler.NewParallel(1)
	defer p.Cleanup()

	p.Add(compiler.Work{
		Result: &binaryPath,
		Name:   "setup-bun",
		Target: "..",
		Source: ".",
	})

	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// runFunc runs the compiled binary with extraEnv applied on top of the
// sandbox's environment, so entries in extraEnv take precedence.
type runFunc func(t *testing.T, extraEnv map[string]string, args ...string) (string, error)

// sandbox returns a sandbox plus a run function bound to its env. It is the
// only way to run the binary in this package, so no test can install into the
// developer's real home directory or reach a cache it did not set up.
func sandbox(t *testing.T, opts ...runner.Opt) (*sdktest.Sandbox, runFunc) {
	t.Helper()

	s := sdktest.NewSandbox(t)
	return s, func(t *testing.T, extraEnv map[string]string, args ...string) (string, error) {
		t.Helper()
		env := s.Env()
		maps.Copy(env, extraEnv)
		r := runner.New(t, binaryPath, append([]runner.Opt{runner.WithSandboxEnv(env)}, opts...)...)
		_, err := r.Run(args...)
		return r.StdoutString() + r.StderrString(), err
	}
}

// stubPATH returns a PATH value with an executable for each of stubs, named by
// its key and holding its value, prepended to the inherited PATH.
func stubPATH(t *testing.T, stubs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, script := range stubs {
		//nolint:gosec // test stub with fixed content; 0700 so it is executable
		assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0700))
	}
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// noBunPATH returns a PATH with the system directories only, so a bun the
// developer has installed can't stand in for one the test expects absent.
func noBunPATH() string {
	return "/usr/bin" + string(os.PathListSeparator) + "/bin"
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	assert.NilError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}
