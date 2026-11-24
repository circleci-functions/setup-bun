// Package bunversion resolves a Bun version spec, such as "1.4", "^1.3" or
// "latest", to a concrete release, and reads such specs from the files
// projects commonly keep them in.
package bunversion

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Latest is the spec for the newest release.
const Latest = "latest"

var exactRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// normalize strips what Bun's own tags and installers put before a version:
// bun-v1.4.2 and v1.4.2 both mean 1.4.2.
func normalize(spec string) string {
	s := strings.TrimSpace(spec)
	s = strings.TrimPrefix(s, "bun-")
	return strings.TrimPrefix(s, "v")
}

// Exact returns spec as a version when it names exactly one release, such as
// 1.4.2, so no version list is needed to resolve it.
func Exact(spec string) (string, bool) {
	s := normalize(spec)
	return s, exactRe.MatchString(s)
}

// Resolve returns the newest of versions matching spec: an exact or partial
// version ("1.4.2", "1.4", "1.x"), a semver range ("^1.3", ">=1.2 <1.4"), or
// "latest" for latest. Pre-releases such as canaries never match a range.
func Resolve(spec string, versions []string, latest string) (string, error) {
	s := strings.ToLower(normalize(spec))
	if s == "" {
		return "", fmt.Errorf("empty version spec")
	}
	if s == Latest {
		if latest == "" {
			return "", fmt.Errorf("the registry tags no latest version")
		}
		return latest, nil
	}

	c, err := semver.NewConstraint(s)
	if err != nil {
		return "", fmt.Errorf("invalid version spec %q: %w", spec, err)
	}
	var best *semver.Version
	for _, raw := range versions {
		v, err := semver.StrictNewVersion(raw)
		if err != nil || !c.Check(v) {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
		}
	}
	if best == nil {
		return "", fmt.Errorf("no Bun release matches %q", spec)
	}
	return best.String(), nil
}
