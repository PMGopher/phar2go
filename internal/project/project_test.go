package project

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PMGopher/phar2go/internal/api"
)

func convertFixture(t *testing.T, input string) *Report {
	t.Helper()
	idx, err := api.Default()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	rep, err := Convert(idx, input, Options{Out: out, Force: true, Module: "example.com/testplugin"})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestConvertPhar(t *testing.T) {
	rep := convertFixture(t, filepath.Join("..", "..", "testdata", "TestPlugin.phar"))
	if rep.Plugin != "TestPlugin" || rep.MainType != "Main" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if rep.TODOs != 0 {
		t.Errorf("%d TODOs, want 0; warnings: %v", rep.TODOs, rep.Warnings)
	}
	for _, want := range []string{"go.mod", "plugin.yml", "plugin.go", "resources/config.yml",
		"internal/testplugin/main.go", "internal/testplugin/event_listener.go", "internal/testplugin/sub_square.go",
		"internal/phpx/array.go", "internal/phpx/regexp2/regexp.go"} {
		if _, err := os.Stat(filepath.Join(rep.Out, want)); err != nil {
			t.Errorf("missing %s", want)
		}
	}
	yml, _ := os.ReadFile(filepath.Join(rep.Out, "plugin.yml"))
	if !strings.Contains(string(yml), "api: 5.0.0") || !strings.Contains(string(yml), "main: testplugin.Main") {
		t.Errorf("plugin.yml not rewritten:\n%s", yml)
	}
	// Every Go file must parse.
	fset := token.NewFileSet()
	filepath.Walk(rep.Out, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".go") {
			if _, err := parser.ParseFile(fset, p, nil, parser.AllErrors); err != nil {
				t.Errorf("%s: %v", p, err)
			}
		}
		return nil
	})
	main, _ := os.ReadFile(filepath.Join(rep.Out, "internal", "testplugin", "main.go"))
	for _, want := range []string{"func (m *Main) OnEnable() (err error)", "RegisterEvents(NewEventListener(m), m)", "phpx.Try("} {
		if !strings.Contains(string(main), want) {
			t.Errorf("main.go doesn't contain %q", want)
		}
	}
	listener, _ := os.ReadFile(filepath.Join(rep.Out, "internal", "testplugin", "event_listener.go"))
	if !strings.Contains(string(listener), `"OnJoin": {"priority": "HIGH"}`) {
		t.Errorf("event handler tags missing:\n%s", listener)
	}
}

func TestConvertFolder(t *testing.T) {
	rep := convertFixture(t, filepath.Join("..", "..", "testdata", "TestPlugin"))
	if rep.Classes != 5 {
		t.Errorf("%d classes, want 5", rep.Classes)
	}
}

// TestBuildAgainstServer builds and runs the converted plugin's test with a pocketmine-go
// checkout (set PHAR2GO_PM to its folder).
func TestBuildAgainstServer(t *testing.T) {
	pm := os.Getenv("PHAR2GO_PM")
	if pm == "" {
		t.Skip("PHAR2GO_PM isn't set")
	}
	rep := convertFixture(t, filepath.Join("..", "..", "testdata", "TestPlugin.phar"))
	abs, _ := filepath.Abs(pm)
	work := "go 1.26.1\n\nuse (\n\t.\n\t" + filepath.ToSlash(abs) + "\n)\n"
	os.WriteFile(filepath.Join(rep.Out, "go.work"), []byte(work), 0o644)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = rep.Out
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test failed: %v\n%s", err, out)
	}
}
