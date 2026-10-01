// Package project turns a PocketMine-MP plugin archive into a pocketmine-go plugin module.
package project

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/PMGopher/phar2go/internal/api"
	"github.com/PMGopher/phar2go/internal/conv"
	"github.com/PMGopher/phar2go/internal/phar"
	"github.com/PMGopher/phar2go/runtime/phpx"
)

// Options configure a conversion.
type Options struct {
	// Out is the output folder. Default: "<PluginName>-go" next to the input.
	Out string
	// Module is the Go module path. Default: github.com/<author>/<plugin name>.
	Module string
	// GoVersion is the go directive of go.mod.
	GoVersion string
	// Force overwrites a non-empty output folder.
	Force bool
}

// Report describes a finished conversion.
type Report struct {
	Out       string
	Plugin    string
	Version   string
	Module    string
	Package   string
	MainType  string
	Files     []string
	Warnings  []string
	TODOs     int
	Classes   int
	Methods   int
	External  []string
	PHPFiles  int
	Resources int
	Commands  []string
}

type pluginYML struct {
	Name     string         `yaml:"name"`
	Main     string         `yaml:"main"`
	Version  any            `yaml:"version"`
	Author   string         `yaml:"author"`
	Authors  any            `yaml:"authors"`
	SrcNS    string         `yaml:"src-namespace-prefix"`
	Commands map[string]any `yaml:"commands"`
}

// Convert converts the plugin archive (or folder) at input.
func Convert(idx *api.Index, input string, opts Options) (*Report, error) {
	arch, err := phar.Open(input)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", input, err)
	}
	root, ymlData, err := findPluginYML(arch)
	if err != nil {
		return nil, err
	}
	var desc pluginYML
	if err := yaml.Unmarshal(ymlData, &desc); err != nil {
		return nil, fmt.Errorf("plugin.yml: %w", err)
	}
	if desc.Name == "" || desc.Main == "" {
		return nil, fmt.Errorf("plugin.yml has no name or main")
	}
	pkg := packageName(desc.Name)
	author := desc.Author
	if author == "" {
		switch a := desc.Authors.(type) {
		case []any:
			if len(a) > 0 {
				author = fmt.Sprint(a[0])
			}
		case string:
			author = a
		}
	}
	module := opts.Module
	if module == "" {
		owner := modulePart(author)
		if owner == "" {
			owner = "you"
		}
		module = "github.com/" + owner + "/" + strings.ToLower(modulePart(desc.Name))
	}
	out := opts.Out
	if out == "" {
		base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		out = filepath.Join(filepath.Dir(input), base+"-go")
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 && !opts.Force {
		return nil, fmt.Errorf("output folder %s isn't empty (use -force to overwrite)", out)
	}
	if opts.Force {
		// Remove what an earlier conversion wrote (keep a .git folder).
		if entries, err := os.ReadDir(out); err == nil {
			for _, e := range entries {
				if e.Name() != ".git" {
					os.RemoveAll(filepath.Join(out, e.Name()))
				}
			}
		}
	}

	// PHP sources and resources.
	phpFiles := map[string][]byte{}
	resources := map[string][]byte{}
	for _, name := range arch.Names() {
		rel, ok := strings.CutPrefix(name, root)
		if !ok {
			continue
		}
		data := arch.Files[name]
		switch {
		case strings.HasPrefix(rel, "resources/"):
			resources[strings.TrimPrefix(rel, "resources/")] = data
		case strings.HasSuffix(rel, ".php") && wantPHP(rel):
			phpFiles[rel] = data
		}
	}
	if len(phpFiles) == 0 {
		return nil, fmt.Errorf("the plugin has no PHP source files")
	}
	mainClass := strings.TrimPrefix(desc.Main, "\\")
	res, err := conv.Convert(idx, phpFiles, conv.Options{
		Package:    pkg,
		PhpxImport: module + "/internal/phpx",
		MainClass:  mainClass,
		PluginName: desc.Name,
	})
	if err != nil {
		return nil, err
	}
	mainType := res.MainType
	if mainType == "" {
		return nil, fmt.Errorf("the main class %s wasn't found in the plugin's code", mainClass)
	}
	rep := &Report{Out: out, Plugin: desc.Name, Version: fmt.Sprint(desc.Version), Module: module, Package: pkg, MainType: mainType,
		Warnings: res.Warnings, TODOs: res.TODOs, Classes: res.Classes, Methods: res.Methods, External: res.External,
		PHPFiles: len(phpFiles), Resources: len(resources)}
	for c := range desc.Commands {
		rep.Commands = append(rep.Commands, c)
	}
	sort.Strings(rep.Commands)

	w := &writer{out: out}
	// The converted code is one package in internal/<package>; the root only registers it.
	for name, data := range res.Files {
		w.write(path.Join("internal", pkg, name), data)
	}
	// plugin.yml with main pointing at the Go type, for pocketmine-go's API.
	yml, apiChanged := rewriteAPI(rewriteMain(ymlData, pkg+"."+mainType))
	if apiChanged {
		rep.Warnings = append([]string{"plugin.yml: api was set to 5.0.0, the API of pocketmine-go"}, rep.Warnings...)
	}
	w.write("plugin.yml", yml)
	for name, data := range resources {
		w.write(path.Join("resources", name), data)
	}
	embed := "plugin.yml"
	if len(resources) > 0 {
		embed += " resources"
	}
	w.write("plugin.go", []byte(pluginGo(pkg, module, desc.Name, mainType, embed, res.UsesServer)))
	w.write("plugin_test.go", []byte(pluginTest(pkg, desc.Name)))
	goVersion := opts.GoVersion
	if goVersion == "" {
		goVersion = "1.26.1"
	}
	w.write("go.mod", []byte(fmt.Sprintf("module %s\n\ngo %s\n", module, goVersion)))
	w.write("go.work", []byte(fmt.Sprintf("go %s\n\n// Development only: builds this plugin against a clone of pocketmine-go next to it.\n// The server doesn't read this file when it builds the plugin.\nuse (\n\t.\n\t../pocketmine-go\n)\n", goVersion)))
	// The phpx runtime.
	fs.WalkDir(phpx.Source, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "embed.go" || strings.HasSuffix(p, "_test.go") {
			return err
		}
		data, _ := phpx.Source.ReadFile(p)
		if strings.HasSuffix(p, ".go") {
			data = []byte(strings.ReplaceAll(string(data), "github.com/PMGopher/phar2go/runtime/phpx", module+"/internal/phpx"))
		}
		w.write(path.Join("internal", "phpx", p), data)
		return nil
	})
	w.write("README.md", []byte(readme(rep)))
	w.write("CONVERSION.md", []byte(conversionReport(rep)))
	if w.err != nil {
		return nil, w.err
	}
	rep.Files = w.files
	return rep, nil
}

type writer struct {
	out   string
	files []string
	err   error
}

func (w *writer) write(name string, data []byte) {
	if w.err != nil {
		return
	}
	p := filepath.Join(w.out, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.err = err
		return
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		w.err = err
		return
	}
	w.files = append(w.files, name)
}

// findPluginYML finds plugin.yml; root is its folder inside the archive ("" or "Plugin/").
func findPluginYML(a *phar.Archive) (string, []byte, error) {
	if d, ok := a.Files["plugin.yml"]; ok {
		return "", d, nil
	}
	best := ""
	for _, n := range a.Names() {
		if path.Base(n) == "plugin.yml" && (best == "" || len(n) < len(best)) {
			best = n
		}
	}
	if best == "" {
		return "", nil, fmt.Errorf("no plugin.yml in the archive: is it a PocketMine-MP plugin?")
	}
	return strings.TrimSuffix(best, "plugin.yml"), a.Files[best], nil
}

// wantPHP filters out tests, tools and libraries that aren't part of the plugin at run time.
func wantPHP(rel string) bool {
	parts := strings.Split(rel, "/")
	// Top-level folders that aren't part of the plugin at run time.
	if len(parts) > 1 {
		switch strings.ToLower(parts[0]) {
		case "tests", "test", ".github", "stubs", "phpstan", "tools", "build", ".phar", "docs", "examples":
			return false
		}
	}
	if strings.HasPrefix(rel, "vendor/") {
		if len(parts) < 3 {
			return false
		}
		switch parts[1] {
		case "composer", "pocketmine", "phpstan", "phpunit", "bin", "symfony", "sebastian", "doctrine", "nikic", "theseer", "myclabs", "phar-io":
			return false
		}
	}
	return rel != "stub.php" && !strings.HasSuffix(rel, "autoload.php")
}

var reMain = regexp.MustCompile(`(?m)^main:\s*.*$`)

func rewriteMain(yml []byte, main string) []byte {
	return reMain.ReplaceAll(yml, []byte("main: "+main+" # converted by phar2go"))
}

var reAPI = regexp.MustCompile(`(?m)^api:.*$`)

// rewriteAPI makes plugin.yml accept pocketmine-go's API version (5.x).
func rewriteAPI(yml []byte) ([]byte, bool) {
	line := reAPI.Find(yml)
	if line == nil {
		return append(yml, []byte("\napi: 5.0.0\n")...), true
	}
	if regexp.MustCompile(`\b5\.\d+\.\d+`).Match(line) {
		return yml, false
	}
	// A list over several lines ("api:\n  - 4.0.0") is replaced too.
	out := reAPI.ReplaceAll(yml, []byte("api: 5.0.0"))
	out = regexp.MustCompile(`(?m)^api: 5\.0\.0\n(\s+-\s*[0-9.]+\s*\n)+`).ReplaceAll(out, []byte("api: 5.0.0\n"))
	return out, true
}

func packageName(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			sb.WriteRune(r)
		}
	}
	s := sb.String()
	if s == "" {
		s = "plugin"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "p" + s
	}
	switch s {
	case "main", "plugin", "server", "player", "phpx", "event", "command", "utils", "world", "item", "block", "math", "scheduler", "form", "test":
		s += "plugin"
	}
	return s
}

func modulePart(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)), r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-._")
}

func pluginGo(pkg, module, name, mainType, embed string, usesServer bool) string {
	create := "return code.New" + mainType + "()"
	if usesServer {
		create = "p := code.New" + mainType + "()\n\t\tcode.PluginInstance = p\n\t\treturn p"
	}
	return fmt.Sprintf(`// Package %s is the %s plugin for pocketmine-go, converted from PocketMine-MP by phar2go.
//
// The plugin's code is in internal/%s. Add the plugin to a server: see README.md.
package %s

import (
	"embed"

	code "%s/internal/%s"
	"pocketmine-go/pocketmine/plugin"
)

// files is the plugin's folder: plugin.yml and the default files in resources/, compiled into
// the server.
//
//go:embed %s
var files embed.FS

// init registers the plugin when the server imports this package.
func init() {
	plugin.RegisterGoPlugin(files, func() plugin.Plugin {
		%s
	})
}
`, pkg, name, pkg, pkg, module, pkg, embed, create)
}

func pluginTest(pkg, name string) string {
	return fmt.Sprintf(`package %s

import (
	"os"
	"path/filepath"
	"testing"

	"pocketmine-go/pocketmine/log"
	"pocketmine-go/pocketmine/server"
)

// TestPluginLoads starts a server in a temporary folder and checks that the plugin is enabled.
func TestPluginLoads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("level-seed=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := server.New(dir, log.NewSimpleLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer s.ForceShutdown()

	p := s.GetPluginManager().GetPlugin(%q)
	if p == nil || !p.IsEnabled() {
		t.Fatal("%s wasn't loaded and enabled")
	}
}
`, pkg, name, name)
}

// Probe reads the plugin's name and the Go package it will get, without converting it.
func Probe(input string) (*Report, error) {
	arch, err := phar.Open(input)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", input, err)
	}
	_, ymlData, err := findPluginYML(arch)
	if err != nil {
		return nil, err
	}
	var desc pluginYML
	if err := yaml.Unmarshal(ymlData, &desc); err != nil {
		return nil, fmt.Errorf("plugin.yml: %w", err)
	}
	return &Report{Plugin: desc.Name, Package: packageName(desc.Name)}, nil
}
