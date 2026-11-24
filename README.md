# setup-bun

Installs [Bun](https://bun.sh) and puts `bun` and `bunx` on `PATH` for the
rest of the job. An optional `install-packages` command then runs a frozen
`bun install`, caching what it downloads.

```yaml
functions:
  bun: github.com/circleci-functions/setup-bun@<version>

jobs:
  test:
    docker:
      - image: cimg/base:stable
    steps:
      - checkout
      - bun:
          with:
            bun-version: "1.4.2"
      - bun/install-packages
      - run: bun test
```

`circleci function add setup-bun --as bun` adds the `functions:` entry at the
latest version. The alias is yours to choose. A step named after it installs
Bun, and `<alias>/install-packages` runs `install-packages`. Each flag below
becomes a `with:` key without its leading `--`, and a step with no flags needs
no `with:` at all.

Bun needs no Node.js. For a project that uses both, install Node.js with
[setup-node](https://github.com/circleci-functions/setup-node) in a step of
its own.

## Installing Bun

With no command, setup-bun downloads Bun's official build for the machine it
runs on from its GitHub release, checks it against the release's
`SHASUMS256.txt`, and links `bun` and `bunx` into a directory already on
`PATH`. Later steps run them by name, in any shell, without sourcing anything.

It links into `~/.local/bin`, `~/bin` or `/usr/local/bin`: the first of them
that is on `PATH` ahead of any `bun` already there and writable without
`sudo`. A preinstalled Bun is then shadowed, not changed. Failing that, it
takes over the directory the preinstalled `bun` is in, keeping the files it
replaces beside them as `<name>.pathlink-old`, with `sudo -n` if that
directory is root-owned.

### Choosing a version

`--bun-version` takes any of:

| Spec | Installs |
|------|----------|
| `1.4.2`, `v1.4.2`, `bun-v1.4.2` | exactly that release |
| `1`, `1.4`, `1.x`, `^1.3`, `>=1.2 <1.4` | the newest release matching |
| `latest` | the newest release |

Canary builds are never installed. Quote versions in YAML (`"1.4"`), or a
version such as `1.10` reaches setup-bun as `1.1`.

Or read the version from a file with `--bun-version-file`:

| File | Read from |
|------|-----------|
| `.bun-version` | the version on its own line |
| `.tool-versions` | the `bun` line |
| `mise.toml` | `tools.bun` |
| `package.json` | `packageManager` (`"bun@1.4.2"`), then `engines.bun` |

With neither flag, setup-bun reads `.bun-version`, then `package.json`, from
the step's working directory. If neither names a version, it uses the `bun`
already on `PATH`, and installs the newest release only if there is none.
`--bun-version` wins when both flags are set.

Exact versions are downloaded straight away. Anything else is resolved against
the versions of the `bun` package in the npm registry, which Bun publishes
under the same numbers as its releases.

### Builds

setup-bun picks the build for the operating system and architecture, and on
Linux the musl build when the system uses musl, as Alpine does. On x64 it
picks the baseline build when the CPU lacks AVX2. Bun's musl build needs the
C++ runtime, which Alpine doesn't ship, so install it first:

```yaml
- run: apk add --no-cache libstdc++ libgcc
- bun
```

### Global installs

Executables from `bun add --global` go in `$BUN_INSTALL/bin`, which is
`~/.bun/bin` unless `$BUN_INSTALL` is set. setup-bun adds that directory to
`PATH` through `$BASH_ENV`, so they resolve by name in later bash steps.

### Mirrors

`--mirror` downloads from any server laid out like
`https://github.com/oven-sh/bun/releases/download`: a `bun-v<version>/`
directory per release holding its zips and `SHASUMS256.txt`. The mirror can
also be set with `$BUN_MIRROR`, so it can live in a context or in project
environment variables. A version other than an exact one is still resolved
against the npm registry.

### Caching the install

The unpacked install is saved to the job cache under a key made of the OS,
architecture, build and Bun version, so later jobs restore it rather than
downloading it again. A cache problem never fails the job. Turn it off with
`--cache=false`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--bun-version` | (see above) | Version spec to install |
| `--bun-version-file` | (empty) | File to read the version spec from |
| `--mirror` | `https://github.com/oven-sh/bun/releases/download` | Server to download Bun from; also read from `$BUN_MIRROR` |
| `--cache` | true | Set `--cache=false` to disable caching the install |

## Installing dependencies

`install-packages` runs `bun install --frozen-lockfile` in `--app-dir`, which
fails if `bun.lock` (or the older binary `bun.lockb`) is out of date with
`package.json`, and caches Bun's package cache around it.

```yaml
- bun/install-packages:
    with:
      app-dir: ./web
```

It needs `bun` on `PATH`, so install Bun in an earlier step. Replace the
command with `--override-command`, which is split on spaces and not run
through a shell:

```yaml
- bun/install-packages:
    with:
      override-command: bun install --frozen-lockfile --production
```

### Caching packages

Caching is on by default. `install-packages` restores the directory
`bun pm cache` reports before installing, and saves it back afterwards, keyed
on the OS, architecture, the cache directory, Bun's version and a checksum of
the lockfile. When nothing is saved for the current lockfile yet, the newest
cache with everything else the same is restored instead, so only the packages
that changed are downloaded.

Nothing is exported to `$BASH_ENV`: later steps that run `bun` find the same
cache directory on their own. Turn caching off with `--cache=false`. Cache
problems never fail the job, and with no lockfile, as an `--override-command`
can run without one, nothing is cached.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--app-dir` | `.` | Directory with `package.json` and the lockfile |
| `--override-command` | (empty) | Command to run instead of `bun install --frozen-lockfile` |
| `--cache` | true | Set `--cache=false` to disable caching packages |

## Platform support

| Platform | Notes |
|----------|-------|
| Linux x64 and arm64 | glibc or musl |
| macOS arm64 and x64 | |
| Windows x64 | `bun` and `bunx` are linked as `.bat` shims, with a script beside each for Git Bash |

## Development

```sh
task ci          # lint, unit tests and a release dry run
task acceptance  # real downloads; needs the network
task smoke-local # the Linux smoke test, in a container
```

See [RELEASE.md](RELEASE.md) for how releases are made.
