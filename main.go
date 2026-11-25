// Command setup-bun installs Bun and puts bun and bunx on PATH for the rest of
// the job, and through its install-packages subcommand runs a frozen bun
// install, caching what it downloads.
//
// Supported platforms: linux, darwin, and windows.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
)

type cli struct {
	Install         installCmd         `cmd:"" default:"withargs" help:"Install Bun and put bun and bunx on PATH. Runs when no command is given."`
	InstallPackages installPackagesCmd `cmd:"" help:"Run a frozen bun install, caching the packages it downloads."`
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-bun"),
		kong.Description("Install Bun and put bun and bunx on PATH for later steps, and optionally run a frozen bun install, caching what each downloads."),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run(ctx)
}
