// Package bundist downloads, verifies and unpacks official Bun builds from a
// server laid out like https://github.com/oven-sh/bun/releases/download, and
// lists Bun's versions from an npm registry.
package bundist

import (
	"fmt"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/cpu"
)

// Target is a Bun build: an operating system and architecture, plus, on
// Linux, whether it links against musl, and on x64, whether it is the
// baseline build for CPUs without AVX2.
type Target struct {
	// OS is Bun's name for the operating system: linux, darwin or windows.
	OS string
	// Arch is Bun's name for the architecture: x64 or aarch64.
	Arch string
	// Musl selects the build for musl libc, as Alpine uses.
	Musl bool
	// Baseline selects the x64 build that runs without AVX2.
	Baseline bool
}

var bunOS = map[string]string{
	"linux":   "linux",
	"darwin":  "darwin",
	"windows": "windows",
}

var bunArch = map[string]string{
	"amd64": "x64",
	"arm64": "aarch64",
}

// muslLoaders matches the dynamic loader a musl system ships, which a glibc
// one does not.
const muslLoaders = "/lib/ld-musl-*.so.1"

// Host returns the build for the machine setup-bun is running on.
func Host() (Target, error) {
	musl := false
	if runtime.GOOS == "linux" {
		matches, _ := filepath.Glob(muslLoaders)
		musl = len(matches) > 0
	}
	return targetFor(runtime.GOOS, runtime.GOARCH, musl, cpu.X86.HasAVX2)
}

func targetFor(goos, goarch string, musl, avx2 bool) (Target, error) {
	os, ok := bunOS[goos]
	if !ok {
		return Target{}, fmt.Errorf("bun has no build for operating system %s", goos)
	}
	arch, ok := bunArch[goarch]
	if !ok {
		return Target{}, fmt.Errorf("bun has no build for architecture %s", goarch)
	}
	return Target{
		OS:       os,
		Arch:     arch,
		Musl:     musl && os == "linux",
		Baseline: arch == "x64" && !avx2,
	}, nil
}

// String returns the target's name in Bun's release assets, e.g.
// linux-x64-musl-baseline.
func (t Target) String() string {
	name := t.OS + "-" + t.Arch
	if t.Musl {
		name += "-musl"
	}
	if t.Baseline {
		name += "-baseline"
	}
	return name
}

// DirName returns the single top-level directory inside the target's archive,
// e.g. bun-linux-x64.
func (t Target) DirName() string {
	return "bun-" + t.String()
}

// ArchiveName returns the file name of the target's build, e.g.
// bun-linux-x64.zip.
func (t Target) ArchiveName() string {
	return t.DirName() + ".zip"
}

// Exe returns name with the target's executable suffix.
func (t Target) Exe(name string) string {
	if t.OS == "windows" {
		return name + ".exe"
	}
	return name
}
