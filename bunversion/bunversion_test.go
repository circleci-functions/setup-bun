package bunversion_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/setup-bun/bunversion"
)

var versions = []string{
	"1.2.23", "1.3.0", "1.3.14", "1.4.0", "1.4.2", "1.4.5",
	"1.4.6-canary.20261124.1", "0.8.1",
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		spec, want string
	}{
		{"1.4.2", "1.4.2"},
		{"v1.4.2", "1.4.2"},
		{"bun-v1.4.2", "1.4.2"},
		{"1.4", "1.4.5"},
		{"1.x", "1.4.5"},
		{"1", "1.4.5"},
		{"^1.3", "1.4.5"},
		{"~1.3", "1.3.14"},
		{">=1.2 <1.4", "1.3.14"},
		{"latest", "1.4.5"},
		{"LATEST", "1.4.5"},
		{"0.8", "0.8.1"},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			got, err := bunversion.Resolve(tc.spec, versions, "1.4.5")
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestResolve_Errors(t *testing.T) {
	_, err := bunversion.Resolve("2", versions, "1.4.5")
	assert.ErrorContains(t, err, `no Bun release matches "2"`)

	_, err = bunversion.Resolve("not a version", versions, "1.4.5")
	assert.ErrorContains(t, err, "invalid version spec")

	_, err = bunversion.Resolve("  ", versions, "1.4.5")
	assert.ErrorContains(t, err, "empty version spec")

	_, err = bunversion.Resolve("latest", versions, "")
	assert.ErrorContains(t, err, "no latest version")
}

func TestExact(t *testing.T) {
	for spec, want := range map[string]bool{
		"1.4.2":      true,
		"v1.4.2":     true,
		"bun-v1.4.2": true,
		" 1.4.2\n":   true,
		"1.4":        false,
		"^1.4.2":     false,
		"latest":     false,
		"1.4.2-rc.1": false,
	} {
		_, got := bunversion.Exact(spec)
		assert.Equal(t, got, want, spec)
	}
	v, _ := bunversion.Exact("bun-v1.4.2")
	assert.Equal(t, v, "1.4.2")
}
