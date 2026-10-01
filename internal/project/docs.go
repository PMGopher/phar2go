package project

import (
	"fmt"
	"strings"
)

func readme(r *Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s for pocketmine-go\n\n", r.Plugin)
	fmt.Fprintf(&sb, "This is the PocketMine-MP plugin **%s** v%s, converted to Go for\n[pocketmine-go](https://github.com/PMGopher/pocketmine-go) by [phar2go](https://github.com/PMGopher/phar2go).\n\n", r.Plugin, r.Version)
	sb.WriteString("## Adding it to your server\n\n")
	sb.WriteString("The easiest way is to let phar2go do it:\n\n```bash\nphar2go -server path/to/pocketmine-go " + r.Plugin + ".phar\n```\n\n")
	sb.WriteString("Or by hand, in your clone of pocketmine-go:\n\n")
	fmt.Fprintf(&sb, "1. Copy this folder into the server's folder, e.g. as `plugins-go/%s`.\n", r.Package)
	fmt.Fprintf(&sb, "2. Tell Go where the module is:\n\n   ```bash\n   go mod edit -require=%s@v0.0.0 -replace=%s=./plugins-go/%s\n   ```\n\n", r.Module, r.Module, r.Package)
	fmt.Fprintf(&sb, "3. Import it in `cmd/pocketmine-go/plugins.go`:\n\n   ```go\n   import _ \"%s\"\n   ```\n\n", r.Module)
	sb.WriteString("4. Rebuild and start the server:\n\n   ```bash\n   go build -o pocketmine-go ./cmd/pocketmine-go\n   ./pocketmine-go\n   ```\n\n")
	if len(r.Commands) > 0 {
		sb.WriteString("## Commands\n\n")
		for _, c := range r.Commands {
			fmt.Fprintf(&sb, "- `/%s`\n", c)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Developing\n\nWith a clone of pocketmine-go next to this folder (see `go.work`):\n\n```bash\ngo vet ./...\ngo test ./...\n```\n\n")
	fmt.Fprintf(&sb, "The converted code is in `internal/%s`, one file per PHP class file. ", r.Package)
	sb.WriteString("`internal/phpx` is the small runtime the converted code uses for PHP's arrays, loose\ncomparisons and standard functions. See [CONVERSION.md](CONVERSION.md) for what to check by hand.\n")
	return sb.String()
}

func conversionReport(r *Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Conversion report: %s\n\n", r.Plugin)
	fmt.Fprintf(&sb, "| | |\n|---|---|\n| PHP files | %d |\n| Classes | %d |\n| Methods and functions | %d |\n| Resources | %d |\n| Main type | `%s.%s` |\n| Places marked `TODO(phar2go)` | %d |\n\n",
		r.PHPFiles, r.Classes, r.Methods, r.Resources, r.Package, r.MainType, r.TODOs)
	if r.TODOs == 0 && len(r.Warnings) == 0 {
		sb.WriteString("Everything was converted. Test the plugin in game before using it in production.\n")
		return sb.String()
	}
	if r.TODOs > 0 {
		sb.WriteString("Code marked `TODO(phar2go)` couldn't be converted; it throws an error if it runs. Search for it:\n\n```bash\ngrep -rn \"TODO(phar2go)\" internal/\n```\n\n")
	}
	if len(r.External) > 0 {
		sb.WriteString("## Classes that pocketmine-go doesn't have\n\nThese come from PocketMine-MP or from libraries (virions) that weren't bundled in the phar.\nCode using them needs to be rewritten:\n\n")
		for _, e := range r.External {
			fmt.Fprintf(&sb, "- `%s`\n", e)
		}
		sb.WriteString("\n")
	}
	if len(r.Warnings) > 0 {
		sb.WriteString("## Notes\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
	}
	return sb.String()
}
