package bundist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	hc "github.com/circleci-functions/toolkit/httpcl"
)

// DefaultRegistry is the npm registry. Bun publishes every release to npm as
// the bun package, under the same version as its GitHub release, so the
// registry lists Bun's versions without GitHub's API rate limit.
const DefaultRegistry = "https://registry.npmjs.org"

// Versions are the Bun versions a registry lists.
type Versions struct {
	// All is every published version, canaries included.
	All []string
	// Latest is the version the registry tags latest.
	Latest string
}

// ListVersions returns the versions of the bun package in the npm registry at
// url.
func ListVersions(ctx context.Context, url string) (Versions, error) {
	url = strings.TrimSuffix(url, "/")
	client := hc.New(ctx, hc.Config{
		Name:    "npm-registry",
		BaseURL: url,
		Timeout: 2 * time.Minute,
	})

	var doc struct {
		DistTags map[string]string          `json:"dist-tags"`
		Versions map[string]json.RawMessage `json:"versions"`
	}
	err := client.Call(ctx, hc.NewRequest(http.MethodGet, "/bun",
		// The abbreviated document npm itself installs from: a fraction of
		// the full one's size.
		hc.Header("Accept", "application/vnd.npm.install-v1+json"),
		hc.JSONDecoder(&doc),
		hc.Timeout(time.Minute),
	))
	if err != nil {
		return Versions{}, fmt.Errorf("list Bun versions from %s/bun: %w", url, err)
	}

	v := Versions{Latest: doc.DistTags["latest"]}
	for version := range doc.Versions {
		v.All = append(v.All, version)
	}
	if len(v.All) == 0 {
		return Versions{}, fmt.Errorf("%s/bun lists no versions", url)
	}
	return v, nil
}
