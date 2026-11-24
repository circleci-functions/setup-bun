package bunversion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// defaultFiles are checked in the working directory, in order, when no version
// is given.
var defaultFiles = []string{".bun-version", "package.json"}

var (
	toolVersionsRe = regexp.MustCompile(`(?m)^bun\s+(\S+)`)
	plainVersionRe = regexp.MustCompile(`(?m)^\s*(\S+)\s*$`)
)

// errNoVersion is returned for a file that exists but names no Bun version.
var errNoVersion = errors.New("no Bun version found")

// FromFile returns the version spec recorded in the file at path. It
// understands package.json (packageManager set to bun@<version>, then
// engines.bun), mise.toml (tools.bun), .tool-versions (a bun line) and files
// holding just a version, such as .bun-version.
func FromFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // reading the caller's chosen version file is the point
	if err != nil {
		return "", err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	var spec string
	switch name := filepath.Base(path); {
	case strings.HasSuffix(name, ".json"):
		spec, err = fromPackageJSON(data)
	case name == ".tool-versions":
		spec = match(toolVersionsRe, data)
	case strings.HasSuffix(name, ".toml"):
		spec, err = fromMise(data)
	default:
		spec = match(plainVersionRe, data)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if spec == "" {
		return "", fmt.Errorf("%s: %w", path, errNoVersion)
	}
	return spec, nil
}

func fromPackageJSON(data []byte) (string, error) {
	var manifest struct {
		PackageManager string `json:"packageManager"`
		Engines        struct {
			Bun string `json:"bun"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	// packageManager may carry a hash after the version: bun@1.4.2+sha512.…
	if pm, ok := strings.CutPrefix(strings.TrimSpace(manifest.PackageManager), "bun@"); ok {
		version, _, _ := strings.Cut(pm, "+")
		return strings.TrimSpace(version), nil
	}
	return strings.TrimSpace(manifest.Engines.Bun), nil
}

func fromMise(data []byte) (string, error) {
	var mise struct {
		Tools map[string]any `toml:"tools"`
	}
	if _, err := toml.Decode(string(data), &mise); err != nil {
		return "", err
	}
	switch bun := mise.Tools["bun"].(type) {
	case string:
		return strings.TrimSpace(bun), nil
	case map[string]any:
		v, _ := bun["version"].(string)
		return strings.TrimSpace(v), nil
	}
	return "", nil
}

func match(re *regexp.Regexp, data []byte) string {
	if m := re.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// Default returns the version spec to use when none is given, and the file it
// came from: the first of .bun-version and package.json in dir that names a
// Bun version. Both are empty when neither does, since a package.json is often
// there for reasons of its own.
func Default(dir string) (spec, source string, err error) {
	for _, name := range defaultFiles {
		path := filepath.Join(dir, name)
		spec, err := FromFile(path)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errNoVersion) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		return spec, path, nil
	}
	return "", "", nil
}
