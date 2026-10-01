package phar

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPhar(t *testing.T) {
	a, err := Open(filepath.Join("..", "..", "testdata", "TestPlugin.phar"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(a.Files["plugin.yml"]), "name: TestPlugin") {
		t.Errorf("plugin.yml not read: %q", a.Files["plugin.yml"])
	}
	if _, ok := a.Files["src/test/plugin/Main.php"]; !ok {
		t.Errorf("files: %v", a.Names())
	}
}
