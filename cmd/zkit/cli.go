package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	version := selfVersion()
	if version == "" {
		version = "(devel)"
	}
	root := &cobra.Command{
		Use:     "zkit",
		Short:   "ZKit project & plugin scaffolder",
		Long:    "zkit scaffolds new projects from the ZKit template and scaffolds or packs compile-time plugins.\n\nRun 'zkit help <command>' for details on a command.",
		Version: version,
	}
	root.SetErrPrefix("zkit:")
	root.SetVersionTemplate("zkit version {{.Version}}\n")
	root.AddCommand(newNewCmd(), newPluginCmd())
	return root
}

func newNewCmd() *cobra.Command {
	var brand, db, from string
	cmd := &cobra.Command{
		Use:   "new <module> [dir]",
		Short: "Scaffold a new project from this template (rename module/brand/db)",
		Long:  "Clone this template into a new directory, rewriting the Go module path, binary/image/volume names, the frontend package name, the brand display name, the default database name, and the localStorage key prefix.\n\n<module> is the new Go module path (e.g. github.com/acme/foo). [dir] defaults to ./<base of module>.\n--brand defaults to the module base name; --db defaults to the sanitized module base name.\n\nTemplate source (auto): --from <dir> uses that dir; `go run` in a checkout uses the cwd; an installed binary checks out the template at its own version into ~/.ZKit/<version>.\nThe proto package name (zerx.v1) is preserved. The new project needs only `go build` (no codegen at creation).",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true // args validated; from here errors are runtime, not usage
			dir := ""
			if len(args) == 2 {
				dir = args[1]
			}
			return runNew(args[0], dir, brand, db, from)
		},
	}
	cmd.Flags().StringVar(&brand, "brand", "", "brand display name (default: module base name)")
	cmd.Flags().StringVar(&db, "db", "", "default database name (default: sanitized module base name)")
	cmd.Flags().StringVar(&from, "from", "", "explicit template root directory (skip the version cache)")
	return cmd
}

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Scaffold or pack a compile-time plugin",
	}
	cmd.AddCommand(newPluginNewCmd(), newPluginPackCmd())
	return cmd
}

func newPluginNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name> [field:type,...]",
		Short: "Scaffold a plugin (proto + impl + frontend page + teardown SQL + all.go anchors)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			spec := ""
			if len(args) == 2 {
				spec = args[1]
			}
			return scaffoldPlugin(args[0], spec)
		},
	}
}

func newPluginPackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pack <name>",
		Short: "Pack an installed plugin into a distributable <name>.zip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return packPlugin(args[0])
		},
	}
}
