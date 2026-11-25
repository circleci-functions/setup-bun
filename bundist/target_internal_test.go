package bundist

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestTargetFor(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch string
		musl, avx2   bool
		want         string
	}{
		{"linux", "amd64", false, true, "linux-x64"},
		{"linux", "amd64", false, false, "linux-x64-baseline"},
		{"linux", "amd64", true, true, "linux-x64-musl"},
		{"linux", "amd64", true, false, "linux-x64-musl-baseline"},
		{"linux", "arm64", false, false, "linux-aarch64"},
		{"linux", "arm64", true, false, "linux-aarch64-musl"},
		{"darwin", "arm64", false, false, "darwin-aarch64"},
		{"darwin", "amd64", false, true, "darwin-x64"},
		{"darwin", "amd64", true, false, "darwin-x64-baseline"},
		{"windows", "amd64", false, true, "windows-x64"},
		{"windows", "amd64", false, false, "windows-x64-baseline"},
		{"windows", "arm64", false, false, "windows-aarch64"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			got, err := targetFor(tc.goos, tc.goarch, tc.musl, tc.avx2)
			assert.NilError(t, err)
			assert.Equal(t, got.String(), tc.want)
			assert.Equal(t, got.ArchiveName(), "bun-"+tc.want+".zip")
		})
	}
}

func TestTargetFor_Unsupported(t *testing.T) {
	_, err := targetFor("freebsd", "amd64", false, true)
	assert.ErrorContains(t, err, "operating system freebsd")
	_, err = targetFor("linux", "386", false, true)
	assert.ErrorContains(t, err, "architecture 386")
}

func TestTarget_Exe(t *testing.T) {
	assert.Equal(t, Target{OS: "windows"}.Exe("bun"), "bun.exe")
	assert.Equal(t, Target{OS: "linux"}.Exe("bun"), "bun")
}
