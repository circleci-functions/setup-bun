package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/toolkit/bashenv"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-bun/bundist"
	"github.com/circleci-functions/setup-bun/bunpath"
	"github.com/circleci-functions/setup-bun/bunversion"
)

// installCmd installs an official Bun build for the running platform and makes
// bun and bunx resolvable by name for the rest of the job.
type installCmd struct {
	BunVersion     string `help:"Bun version to install: an exact or partial version (1.4.2, 1.4), a semver range (^1.3), or latest. Defaults to .bun-version, then the packageManager or engines.bun entry in package.json, then the Bun already on PATH, then latest." example:"1.4.2"`
	BunVersionFile string `help:"File to read the Bun version from: .bun-version, .tool-versions, mise.toml or package.json." type:"path"`
	Mirror         string `help:"Base URL of a server laid out like https://github.com/oven-sh/bun/releases/download to download Bun from. The mirror can also be set via the BUN_MIRROR env var." default:"https://github.com/oven-sh/bun/releases/download" env:"BUN_MIRROR"`
	Cache          bool   `help:"Restore the Bun install from the job cache, and save it after a fresh download. Set to false to disable caching." default:"true"`
	Registry       string `help:"npm registry to list Bun's versions from." default:"https://registry.npmjs.org" hidden:""`
	InstallDir     string `help:"Directory to install Bun versions under." default:"~/.cache/setup-bun" hidden:""`
	BinDir         string `help:"Link bun and bunx into this directory instead of discovering one on PATH." type:"path" hidden:""`
	CacheNonce     string `help:"Partition the Bun install cache, so a test run cannot restore an earlier run's install." hidden:""`
}

func (c *installCmd) Run(ctx context.Context, client *sdk.Client) error {
	spec, err := c.versionSpec()
	if err != nil {
		return err
	}
	if spec == "" {
		if version, ok := onPath(); ok {
			fmt.Fprintf(os.Stderr, "setup-bun: using the Bun %s already on PATH; pass --bun-version to install another\n", version)
			c.addGlobalBinDir()
			return nil
		}
		fmt.Fprintf(os.Stderr, "setup-bun: no --bun-version, .bun-version or Bun pin in package.json, and no bun on PATH; using %s\n", bunversion.Latest)
		spec = bunversion.Latest
	}

	target, err := bundist.Host()
	if err != nil {
		return err
	}
	installDir, err := expandHome(c.InstallDir)
	if err != nil {
		return err
	}

	version, exact := bunversion.Exact(spec)
	if !exact {
		if version, err = c.resolve(ctx, spec, target); err != nil {
			return err
		}
	}
	root := filepath.Join(installDir, version+"-"+target.String())
	if !c.restore(ctx, client, version, target, root) {
		if err := c.download(ctx, client, version, target, root); err != nil {
			return err
		}
	}

	dir, err := bunpath.Link(bunpath.Options{BinDir: bundist.BinDir(root), Version: version, LinkDir: c.BinDir})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-bun: Bun %s installed at %s, linked into %s\n", version, root, dir)
	c.addGlobalBinDir()
	return nil
}

// resolve returns the newest Bun release matching spec.
func (c *installCmd) resolve(ctx context.Context, spec string, target bundist.Target) (string, error) {
	versions, err := bundist.ListVersions(ctx, c.Registry)
	if err != nil {
		return "", err
	}
	version, err := bunversion.Resolve(spec, versions.All, versions.Latest)
	if err != nil {
		return "", fmt.Errorf("resolve Bun version: %w", err)
	}
	fmt.Fprintf(os.Stderr, "setup-bun: resolved %q to Bun %s (%s)\n", spec, version, target)
	return version, nil
}

// versionSpec picks the version spec from --bun-version, --bun-version-file,
// or the default version files in the working directory, in that order. It
// is empty when none of them names a version.
func (c *installCmd) versionSpec() (string, error) {
	switch {
	case c.BunVersion != "":
		if c.BunVersionFile != "" {
			fmt.Fprintf(os.Stderr, "setup-bun: both --bun-version and --bun-version-file are set; using --bun-version\n")
		}
		return c.BunVersion, nil
	case c.BunVersionFile != "":
		spec, err := bunversion.FromFile(c.BunVersionFile)
		if err != nil {
			return "", fmt.Errorf("bun-version-file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "setup-bun: read Bun version %q from %s\n", spec, c.BunVersionFile)
		return spec, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	spec, source, err := bunversion.Default(wd)
	if err != nil {
		return "", err
	}
	if source != "" {
		fmt.Fprintf(os.Stderr, "setup-bun: read Bun version %q from %s\n", spec, source)
	}
	return spec, nil
}

// restore reports whether root holds an install, from an earlier run or,
// failing that, the job cache.
func (c *installCmd) restore(ctx context.Context, client *sdk.Client, version string, target bundist.Target, root string) bool {
	if installed(root, target) {
		return true
	}
	if !c.Cache {
		return false
	}
	_, err := client.RestoreCache(ctx, []string{installCacheKey(version, target, root, c.CacheNonce)}, sdk.WithStepName("Restoring Bun "+version))
	sdk.LogCacheErr("restore", err)
	if !installed(root, target) {
		return false
	}
	fmt.Fprintf(os.Stderr, "setup-bun: restored Bun %s from cache\n", version)
	return true
}

// download installs version at root, and saves it to the job cache.
func (c *installCmd) download(ctx context.Context, client *sdk.Client, version string, target bundist.Target, root string) error {
	if err := bundist.New(ctx, c.Mirror).Install(ctx, version, target, root); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-bun: downloaded Bun %s (%s) from %s\n", version, target, c.Mirror)
	if c.Cache {
		sdk.LogCacheErr("save", client.SaveCache(ctx, installCacheKey(version, target, root, c.CacheNonce), []string{root}, sdk.WithStepName("Saving Bun "+version)))
	}
	return nil
}

// addGlobalBinDir puts the directory bun add --global installs into on PATH
// for later bash steps. It is a convenience, so failing is only a warning.
func (c *installCmd) addGlobalBinDir() {
	dir, err := bunpath.AddGlobalBinDir()
	switch {
	case errors.Is(err, bashenv.ErrNoBashEnv):
		// Not in a job: there are no later steps to reach.
	case err != nil:
		fmt.Fprintf(os.Stderr, "setup-bun: warning: %v; executables from bun add --global will not be on PATH\n", err)
	default:
		fmt.Fprintf(os.Stderr, "setup-bun: added %s, where bun add --global installs executables, to PATH for later bash steps\n", dir)
	}
}

// onPath returns the version of the bun on PATH, if there is one that runs.
func onPath() (string, bool) {
	bin, err := exec.LookPath("bun")
	if err != nil {
		return "", false
	}
	version, err := bunpath.ReportedVersion(bin)
	return version, err == nil && version != ""
}

// installCacheKey is the job cache key for an install of version for target
// at root. root is in the key because a cache restores to the path it was
// saved from: a job whose $HOME differs would otherwise restore an install
// somewhere it never looks.
func installCacheKey(version string, target bundist.Target, root, nonce string) string {
	rootHash := sha256.Sum256([]byte(root))
	key := fmt.Sprintf("setup-bun-install-v1-%s-%s-%s-%x", sdk.OSArch(), target, version, rootHash[:4])
	if nonce != "" {
		key += "-" + nonce
	}
	return key
}

// expandHome resolves a leading ~ against $HOME. kong's path type resolves it
// against the passwd entry instead, which ignores a redirected $HOME.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return filepath.Abs(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

func installed(root string, target bundist.Target) bool {
	_, err := os.Stat(filepath.Join(bundist.BinDir(root), target.Exe("bun")))
	return err == nil
}
