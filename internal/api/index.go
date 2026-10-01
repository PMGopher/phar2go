package api

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Index is the API of a pocketmine-go version.
type Index struct {
	// Module is the server's module path ("pocketmine-go").
	Module string `json:"module"`
	// Version describes the checkout the index was generated from.
	Version  string              `json:"version"`
	Packages map[string]*Package `json:"packages"`
	// PHPClasses maps a lower-case PHP class name (pocketmine\player\Player) to the Go type it is
	// a port of ("pocketmine-go/pocketmine/player.Player").
	PHPClasses map[string]string `json:"php_classes"`
	// PHPMembers maps "lower-case class::member" to a Go function ("pkg.Func") or method
	// ("pkg.Type.Method") that is a port of it.
	PHPMembers map[string]string `json:"php_members"`
	// PHPConsts are constants of PocketMine-MP classes ("lower-case class::NAME") with literal
	// values, used when pocketmine-go has no Go constant for them.
	PHPConsts map[string]PHPConst `json:"php_consts,omitempty"`
}

// Package is one Go package of the server.
type Package struct {
	Path   string               `json:"path"`
	Name   string               `json:"name"`
	Types  map[string]*TypeInfo `json:"types,omitempty"`
	Funcs  map[string]*Func     `json:"funcs,omitempty"`
	Consts map[string]*Const    `json:"consts,omitempty"`
	Vars   map[string]*Type     `json:"vars,omitempty"`
}

// TypeInfo is a named type.
type TypeInfo struct {
	Underlying *Type    `json:"u"`
	Interface  bool     `json:"i,omitempty"`
	TypeParams []string `json:"tp,omitempty"`
	// Methods is the method set of *T (of T for interfaces), promoted methods included.
	Methods map[string]*Func `json:"m,omitempty"`
	// ValueMethods lists the methods in the method set of T itself (value receivers).
	ValueMethods []string `json:"vm,omitempty"`
	// Fields are the exported fields, promoted ones included.
	Fields map[string]*Type `json:"f,omitempty"`
	// Embedded lists the embedded fields' types.
	Embedded []*Type `json:"em,omitempty"`
	// Unexported is set when the method set has unexported methods (the type can only be
	// implemented by embedding one of the package's types).
	Unexported bool `json:"ux,omitempty"`
	// PHP is the PHP class this is a port of.
	PHP string `json:"php,omitempty"`
}

// Const is a constant.
type Const struct {
	Type  *Type  `json:"t"`
	Value string `json:"v,omitempty"`
}

//go:embed index.json.gz
var embedded []byte

// Default returns the index embedded in phar2go.
func Default() (*Index, error) {
	if len(embedded) == 0 {
		return nil, fmt.Errorf("phar2go was built without an API index: run `phar2go index -pm <pocketmine-go>` first")
	}
	return Decode(bytes.NewReader(embedded))
}

// Load reads an index file written by Save (gzip compressed or not).
func Load(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// Decode reads an index.
func Decode(r io.Reader) (*Index, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if data, err = io.ReadAll(zr); err != nil {
			return nil, err
		}
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode API index: %w", err)
	}
	return &idx, nil
}

// Save writes the index gzip compressed.
func (idx *Index) Save(path string) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(data)
	zw.Close()
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Type returns the named type "pkg.Name".
func (idx *Index) Type(name string) *TypeInfo {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return nil
	}
	p := idx.Packages[name[:i]]
	if p == nil {
		return nil
	}
	return p.Types[name[i+1:]]
}

// Func returns the package-level function "pkg.Name".
func (idx *Index) Func(name string) *Func {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return nil
	}
	p := idx.Packages[name[:i]]
	if p == nil {
		return nil
	}
	return p.Funcs[name[i+1:]]
}

// PackagePaths returns every package path, sorted.
func (idx *Index) PackagePaths() []string {
	out := make([]string, 0, len(idx.Packages))
	for p := range idx.Packages {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
