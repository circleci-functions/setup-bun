package bunversion_test

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/setup-bun/bunversion"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	assert.NilError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func TestFromFile(t *testing.T) {
	for _, tc := range []struct {
		name, file, content, want string
	}{
		{"bun-version", ".bun-version", "1.4.2\n", "1.4.2"},
		{"bun-version CRLF", ".bun-version", "1.4\r\n", "1.4"},
		{"bun-version single number", ".bun-version", "1\n", "1"},
		{"any plain file", "version.txt", "  ^1.3  \n", "^1.3"},
		{"tool-versions", ".tool-versions", "nodejs 22.11.0\nbun 1.4.2\n", "1.4.2"},
		{"packageManager", "package.json", `{"packageManager": "bun@1.4.2"}`, "1.4.2"},
		{"packageManager with hash", "package.json", `{"packageManager": "bun@1.4.2+sha512.abc"}`, "1.4.2"},
		{"packageManager wins over engines", "package.json", `{"packageManager": "bun@1.4.2", "engines": {"bun": ">=1.3"}}`, "1.4.2"},
		{"engines.bun", "package.json", `{"packageManager": "pnpm@10.0.0", "engines": {"bun": ">=1.3"}}`, ">=1.3"},
		{"mise string", "mise.toml", "[tools]\nbun = \"1.4\"\n", "1.4"},
		{"mise table", "mise.toml", "[tools]\nbun = { version = \"1.4.2\" }\n", "1.4.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, t.TempDir(), tc.file, tc.content)
			got, err := bunversion.FromFile(path)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestFromFile_NoVersion(t *testing.T) {
	for _, tc := range []struct {
		file, content string
	}{
		{"package.json", `{"name": "app"}`},
		{".tool-versions", "nodejs 22.11.0\n"},
		{"mise.toml", "[tools]\nnode = \"22\"\n"},
		{".bun-version", "\n"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			_, err := bunversion.FromFile(write(t, t.TempDir(), tc.file, tc.content))
			assert.ErrorContains(t, err, "no Bun version found")
		})
	}
}

func TestFromFile_Missing(t *testing.T) {
	_, err := bunversion.FromFile(filepath.Join(t.TempDir(), ".bun-version"))
	assert.Assert(t, os.IsNotExist(err), "got %v", err)
}

func TestDefault(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		spec, source, err := bunversion.Default(t.TempDir())
		assert.NilError(t, err)
		assert.Equal(t, spec, "")
		assert.Equal(t, source, "")
	})

	t.Run("package.json without a Bun pin is skipped", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "package.json", `{"name": "app"}`)
		spec, source, err := bunversion.Default(dir)
		assert.NilError(t, err)
		assert.Equal(t, spec, "")
		assert.Equal(t, source, "")
	})

	t.Run(".bun-version wins over package.json", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "package.json", `{"packageManager": "bun@1.3.0"}`)
		want := write(t, dir, ".bun-version", "1.4.2\n")
		spec, source, err := bunversion.Default(dir)
		assert.NilError(t, err)
		assert.Equal(t, spec, "1.4.2")
		assert.Equal(t, source, want)
	})

	t.Run("package.json", func(t *testing.T) {
		dir := t.TempDir()
		want := write(t, dir, "package.json", `{"packageManager": "bun@1.3.0"}`)
		spec, source, err := bunversion.Default(dir)
		assert.NilError(t, err)
		assert.Equal(t, spec, "1.3.0")
		assert.Equal(t, source, want)
	})

	t.Run("malformed package.json fails", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "package.json", `{`)
		_, _, err := bunversion.Default(dir)
		assert.ErrorContains(t, err, "package.json")
	})
}
