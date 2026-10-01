// Command phar2go converts PocketMine-MP plugins (.phar) into Go plugins for pocketmine-go.
//
//	phar2go MyPlugin.phar                         writes MyPlugin-go/, a Go plugin module
//	phar2go -server ../pocketmine-go MyPlugin.phar   also adds it to a pocketmine-go clone and builds it
//	phar2go -check ../pocketmine-go MyPlugin.phar    builds the result against a pocketmine-go clone
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/PMGopher/phar2go/internal/api"
	"github.com/PMGopher/phar2go/internal/project"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func usage() {
	fmt.Fprintf(os.Stderr, `phar2go %s: convert PocketMine-MP plugins (.phar) to Go plugins for pocketmine-go

Usage:
  phar2go [options] <plugin.phar | plugin folder>...
  phar2go index -pm <pocketmine-go folder> [-o index.json.gz]

Options:
`, version)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Examples:
  phar2go MyPlugin.phar
      Writes the Go plugin to MyPlugin-go/.
  phar2go -server ../pocketmine-go MyPlugin.phar
      Converts, adds the plugin to your pocketmine-go clone and builds the server.
`)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "index" {
		indexCmd(os.Args[2:])
		return
	}
	out := flag.String("o", "", "output folder (default: <name>-go next to the input)")
	module := flag.String("module", "", "Go module path of the plugin (default: github.com/<author>/<name>)")
	force := flag.Bool("force", false, "overwrite the output folder")
	server := flag.String("server", "", "pocketmine-go folder: install the plugin into it and build the server")
	check := flag.String("check", "", "pocketmine-go folder: build and vet the converted plugin against it")
	indexFile := flag.String("index", "", "use this API index instead of the built-in one (see `phar2go index`)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	pause := flag.Bool("pause", runtime.GOOS == "windows" && len(os.Args) == 2 && !strings.HasPrefix(os.Args[1], "-"), "wait for Enter before exiting")
	flag.Usage = usage
	flag.Parse()
	if *showVersion {
		fmt.Println("phar2go", version)
		return
	}
	code := run(flag.Args(), *out, *module, *force, *server, *check, *indexFile)
	if *pause {
		fmt.Print("\nPress Enter to exit...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code)
}

func run(inputs []string, out, module string, force bool, server, check, indexFile string) int {
	if len(inputs) == 0 {
		usage()
		return 2
	}
	if len(inputs) > 1 && out != "" {
		fmt.Fprintln(os.Stderr, "-o can only be used with one input")
		return 2
	}
	var idx *api.Index
	var err error
	if indexFile != "" {
		idx, err = api.Load(indexFile)
	} else {
		idx, err = api.Default()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	failed := 0
	for _, in := range inputs {
		if err := convertOne(idx, in, out, module, force, server, check); err != nil {
			fmt.Fprintf(os.Stderr, "\n✗ %s: %v\n", in, err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func convertOne(idx *api.Index, in, out, module string, force bool, server, check string) error {
	opts := project.Options{Out: out, Module: module, Force: force}
	if server != "" {
		abs, err := filepath.Abs(server)
		if err != nil {
			return err
		}
		server = abs
		if _, err := os.Stat(filepath.Join(server, "cmd", "pocketmine-go")); err != nil {
			return fmt.Errorf("%s isn't a pocketmine-go folder (no cmd/pocketmine-go)", server)
		}
	}
	if server != "" && out == "" {
		// Install into the server: <server>/plugins-go/<package>.
		probe, err := project.Probe(in)
		if err != nil {
			return err
		}
		opts.Out = filepath.Join(server, "plugins-go", probe.Package)
		opts.Force = true
	}
	fmt.Printf("Converting %s...\n", in)
	rep, err := project.Convert(idx, in, opts)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s v%s -> %s\n", rep.Plugin, rep.Version, rep.Out)
	fmt.Printf("  %d classes, %d methods from %d PHP files; module %s\n", rep.Classes, rep.Methods, rep.PHPFiles, rep.Module)
	if rep.TODOs > 0 {
		fmt.Printf("  ! %d place(s) need a manual look: see CONVERSION.md (search for TODO(phar2go))\n", rep.TODOs)
	}
	if len(rep.External) > 0 {
		fmt.Printf("  ! uses %d class(es) pocketmine-go doesn't have: %s\n", len(rep.External), strings.Join(rep.External, ", "))
	}
	if server != "" {
		return install(server, rep)
	}
	if check != "" {
		return checkBuild(check, rep)
	}
	fmt.Printf("  Next: phar2go -server <pocketmine-go folder> %s   (adds it to your server)\n", in)
	return nil
}

// install adds the converted plugin to a pocketmine-go clone and builds the server.
func install(server string, rep *project.Report) error {
	os.Remove(filepath.Join(rep.Out, "go.work"))
	rel, err := filepath.Rel(server, rep.Out)
	if err != nil {
		return err
	}
	rel = "./" + filepath.ToSlash(rel)
	if err := goCmd(server, "mod", "edit", "-require="+rep.Module+"@v0.0.0", "-replace="+rep.Module+"="+rel); err != nil {
		return err
	}
	pluginsGo := filepath.Join(server, "cmd", "pocketmine-go", "plugins.go")
	src, err := os.ReadFile(pluginsGo)
	if err != nil {
		return err
	}
	imp := fmt.Sprintf("import _ %q // %s, converted by phar2go", rep.Module, rep.Plugin)
	if !strings.Contains(string(src), fmt.Sprintf("%q", rep.Module)) {
		s := string(src)
		i := strings.Index(s, "package main")
		if i < 0 {
			return fmt.Errorf("%s has no package clause", pluginsGo)
		}
		end := i + len("package main")
		s = s[:end] + "\n\n" + imp + s[end:]
		if err := os.WriteFile(pluginsGo, []byte(s), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("  Added to %s (go.mod and cmd/pocketmine-go/plugins.go)\n", server)
	fmt.Println("  Building the server...")
	exe := "pocketmine-go"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if err := goCmd(server, "build", "-o", exe, "./cmd/pocketmine-go"); err != nil {
		return fmt.Errorf("the server doesn't build with the plugin; see the errors above and %s", filepath.Join(rep.Out, "CONVERSION.md"))
	}
	fmt.Printf("✓ Done: start the server with %s\n", filepath.Join(server, exe))
	return nil
}

// checkBuild builds and vets the converted plugin against a pocketmine-go clone.
func checkBuild(pm string, rep *project.Report) error {
	abs, err := filepath.Abs(pm)
	if err != nil {
		return err
	}
	work := fmt.Sprintf("go 1.26.1\n\nuse (\n\t.\n\t%s\n)\n", filepath.ToSlash(abs))
	if err := os.WriteFile(filepath.Join(rep.Out, "go.work"), []byte(work), 0o644); err != nil {
		return err
	}
	fmt.Println("  Building against", abs, "...")
	if err := goCmd(rep.Out, "build", "./..."); err != nil {
		return errors.New("the converted plugin doesn't build")
	}
	if err := goCmd(rep.Out, "vet", "./..."); err != nil {
		fmt.Println("  ! go vet reported problems (the plugin builds)")
	}
	fmt.Println("✓ The converted plugin builds")
	return nil
}

func goCmd(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("the go command wasn't found: install Go from https://go.dev/dl/")
		}
		return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func indexCmd(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	pm := fs.String("pm", "../pocketmine-go", "pocketmine-go folder")
	out := fs.String("o", "index.json.gz", "output file")
	phpxDir := fs.String("phpx", "./runtime/phpx", "folder of the phpx runtime (in the phar2go source)")
	fs.Parse(args)
	idx, err := api.Generate(*pm, *phpxDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := idx.Save(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d packages, %d PHP classes, %d PHP members\n", *out, len(idx.Packages), len(idx.PHPClasses), len(idx.PHPMembers))
}
