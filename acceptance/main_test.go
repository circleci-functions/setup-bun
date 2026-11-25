//go:build acceptance

// Package setupbun_test exercises setup-bun against real infrastructure: real
// Bun releases and the npm registry, real bun install runs, and a real CircleCI
// task-agent cache when running inside a job. Tagged out of `task test`
// because it needs the network. The CLI contract itself is covered
// hermetically by clitest/.
package setupbun_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// binaryPathEnvVar names a prebuilt setup-bun to test, for an executor with no
// Go toolchain to compile one with. The smoke jobs set it, and pair it with a
// cross-compiled copy of this suite. A relative path is resolved here, since
// the suite runs the binary from a working directory of its own.
const binaryPathEnvVar = "SETUP_BUN_BINARY"

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
	if bin := os.Getenv(binaryPathEnvVar); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		binaryPath = abs
		return m.Run(), nil
	}

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

// acceptanceVersion returns the Bun version spec to install:
// SETUP_BUN_ACCEPTANCE_VERSION is set in CI from the pipeline's bun-version
// parameter, latest locally.
func acceptanceVersion() string {
	if v := os.Getenv("SETUP_BUN_ACCEPTANCE_VERSION"); v != "" {
		return v
	}
	return "latest"
}

var exactVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// runFunc runs the compiled binary with extraEnv applied on top of the
// sandbox's environment, so entries in extraEnv take precedence.
type runFunc func(t *testing.T, extraEnv map[string]string, args ...string) (string, error)

// sandbox returns a sandbox backed by an in-memory cache, plus a run function
// bound to its env. bun resolves its cache under $HOME, which the sandbox
// points at a temp dir, so an install never touches the real one.
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

// reported returns the first submatch of pattern in setup-bun's output.
func reported(t *testing.T, out, pattern string) string {
	t.Helper()
	re := regexp.MustCompile(pattern)
	for line := range strings.SplitSeq(out, "\n") {
		if m := re.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	t.Fatalf("setup-bun did not report %q:\n%s", pattern, out)
	return ""
}
