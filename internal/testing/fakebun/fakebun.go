// Package fakebun serves stand-ins for Bun's release downloads and for the npm
// registry's bun package, so tests can resolve, download and unpack Bun
// builds without the network.
package fakebun

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// DefaultTargets are the builds a release ships when it sets no Targets.
var DefaultTargets = []string{
	"linux-x64", "linux-x64-baseline", "linux-x64-musl", "linux-x64-musl-baseline",
	"linux-aarch64", "linux-aarch64-musl",
	"darwin-x64", "darwin-x64-baseline", "darwin-aarch64",
	"windows-x64", "windows-x64-baseline",
}

// Release is one Bun release the server lists.
type Release struct {
	Version string
	// Targets are the builds it ships; nil means DefaultTargets.
	Targets []string
}

// Server is a running fake release server and registry, on one URL: releases
// under /bun-v<version>/, and the registry document at /bun.
type Server struct {
	URL string

	releases []Release
	latest   string
	mu       sync.Mutex
	requests []string
	corrupt  bool
}

// New starts a server listing releases, the last of which it tags latest, and
// stops it when t ends.
func New(t *testing.T, releases ...Release) *Server {
	t.Helper()
	s := &Server{releases: releases}
	if len(releases) > 0 {
		s.latest = releases[len(releases)-1].Version
	}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// Corrupt makes the server send archives that do not match their checksums.
func (s *Server) Corrupt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.corrupt = true
}

// Requests returns the paths served so far.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

// Downloads returns how many archives have been served.
func (s *Server) Downloads() int {
	n := 0
	for _, p := range s.Requests() {
		if strings.HasSuffix(p, ".zip") {
			n++
		}
	}
	return n
}

// RegistryRequests returns how many times the version list has been fetched.
func (s *Server) RegistryRequests() int {
	return len(slices.DeleteFunc(s.Requests(), func(p string) bool { return p != "/bun" }))
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.URL.Path)
	corrupt := s.corrupt
	s.mu.Unlock()

	if r.URL.Path == "/bun" {
		s.serveRegistry(w)
		return
	}

	dir, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	rel, ok := s.release(dir)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if name == "SHASUMS256.txt" {
		for _, target := range targets(rel) {
			_, _ = fmt.Fprintf(w, "%x  bun-%s.zip\n", sha256.Sum256(Archive(rel.Version, target)), target)
		}
		return
	}
	target, ok := strings.CutSuffix(strings.TrimPrefix(name, "bun-"), ".zip")
	if !ok || !slices.Contains(targets(rel), target) {
		http.NotFound(w, r)
		return
	}
	data := Archive(rel.Version, target)
	if corrupt {
		data = append(data, 0)
	}
	_, _ = w.Write(data)
}

func (s *Server) serveRegistry(w http.ResponseWriter) {
	versions := map[string]any{}
	for _, rel := range s.releases {
		versions[rel.Version] = map[string]string{"name": "bun", "version": rel.Version}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":      "bun",
		"dist-tags": map[string]string{"latest": s.latest},
		"versions":  versions,
	})
}

func (s *Server) release(dir string) (Release, bool) {
	for _, rel := range s.releases {
		if "bun-v"+rel.Version == dir {
			return rel, true
		}
	}
	return Release{}, false
}

func targets(rel Release) []string {
	if rel.Targets != nil {
		return rel.Targets
	}
	return DefaultTargets
}

// Archive returns the zip of version's build for target, laid out like a real
// one: a single bun-<target>/ directory holding bun, or bun.exe on Windows.
// On Unix, bun is a shell script that prints version, so a test can run it.
// Entries are written in a fixed order, so the checksum is the same every time.
func Archive(version, target string) []byte {
	name, script := "bun", "#!/bin/sh\necho "+version+"\n"
	if strings.HasPrefix(target, "windows-") {
		name, script = "bun.exe", version
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "bun-" + target + "/" + name, Method: zip.Deflate}
	h.SetMode(0755)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte(script))
	_ = zw.Close()
	return buf.Bytes()
}
