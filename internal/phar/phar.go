// Package phar reads PHP archives (.phar), in the phar, zip and tar formats, and plugin folders.
package phar

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Archive is the content of a phar: file paths (with forward slashes, relative to the archive
// root) to their contents.
type Archive struct {
	Files map[string][]byte
	// Stub is the PHP stub of a phar-format archive.
	Stub []byte
	// Alias is the phar alias.
	Alias string
}

// Names returns the file paths, sorted.
func (a *Archive) Names() []string {
	out := make([]string, 0, len(a.Files))
	for n := range a.Files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Open reads a .phar file, a .zip/.tar(.gz) archive, or a plugin folder.
func Open(p string) (*Archive, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return readDir(p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return Read(data)
}

// Read reads an archive from memory, detecting its format.
func Read(data []byte) (*Archive, error) {
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return readZip(data)
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(zr)
		if err != nil {
			return nil, err
		}
		return Read(raw)
	case bytes.HasPrefix(data, []byte("BZh")):
		raw, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(data)))
		if err != nil {
			return nil, err
		}
		return Read(raw)
	case len(data) > 262 && string(data[257:262]) == "ustar":
		return readTar(data)
	}
	if i := bytes.Index(data, []byte("__HALT_COMPILER();")); i >= 0 {
		return readPhar(data, i)
	}
	// A zip phar with a stub in front of it.
	if i := bytes.LastIndex(data, []byte("PK\x05\x06")); i >= 0 {
		if a, err := readZip(data); err == nil {
			return a, nil
		}
	}
	return nil, errors.New("not a phar, zip or tar archive")
}

func readPhar(data []byte, haltAt int) (*Archive, error) {
	pos := haltAt + len("__HALT_COMPILER();")
	// The stub ends with "__HALT_COMPILER(); ?>" and an optional newline.
	rest := data[pos:]
	if bytes.HasPrefix(bytes.TrimLeft(rest, " "), []byte("?>")) {
		pos += bytes.Index(rest, []byte("?>")) + 2
		if bytes.HasPrefix(data[pos:], []byte("\r\n")) {
			pos += 2
		} else if bytes.HasPrefix(data[pos:], []byte("\n")) {
			pos++
		}
	}
	a := &Archive{Files: map[string][]byte{}, Stub: data[:pos]}
	r := &reader{b: data, pos: pos}
	manifestLen := int(r.u32())
	manifestStart := r.pos
	count := int(r.u32())
	r.u16() // API version
	globalFlags := r.u32()
	_ = globalFlags
	a.Alias = string(r.bytes(int(r.u32())))
	r.bytes(int(r.u32())) // metadata
	if r.err != nil {
		return nil, fmt.Errorf("bad phar manifest: %w", r.err)
	}
	type entry struct {
		name           string
		size, csize    uint32
		crc, flags     uint32
	}
	entries := make([]entry, 0, count)
	for i := 0; i < count; i++ {
		var e entry
		e.name = string(r.bytes(int(r.u32())))
		e.size = r.u32()
		r.u32() // timestamp
		e.csize = r.u32()
		e.crc = r.u32()
		e.flags = r.u32()
		r.bytes(int(r.u32())) // metadata
		if r.err != nil {
			return nil, fmt.Errorf("bad phar manifest entry %d: %w", i, r.err)
		}
		entries = append(entries, e)
	}
	r.pos = manifestStart + manifestLen
	for _, e := range entries {
		raw := r.bytes(int(e.csize))
		if r.err != nil {
			return nil, fmt.Errorf("phar truncated reading %s", e.name)
		}
		content, err := decompress(raw, e.flags, e.size)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.name, err)
		}
		if e.crc != 0 && crc32.ChecksumIEEE(content) != e.crc {
			return nil, fmt.Errorf("%s: CRC mismatch", e.name)
		}
		name := cleanName(e.name)
		if name == "" || strings.HasSuffix(e.name, "/") {
			continue
		}
		a.Files[name] = content
	}
	return a, nil
}

func decompress(raw []byte, flags uint32, size uint32) ([]byte, error) {
	switch {
	case flags&0x1000 != 0: // gzip (raw deflate)
		out, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
		if err != nil {
			return nil, fmt.Errorf("inflate: %w", err)
		}
		return out, nil
	case flags&0x2000 != 0: // bzip2
		out, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(raw)))
		if err != nil {
			return nil, fmt.Errorf("bunzip2: %w", err)
		}
		return out, nil
	}
	return raw, nil
}

func readZip(data []byte) (*Archive, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	a := &Archive{Files: map[string][]byte{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if n := cleanName(f.Name); n != "" && !strings.HasPrefix(n, ".phar/") {
			a.Files[n] = b
		}
	}
	return a, nil
}

func readTar(data []byte) (*Archive, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	a := &Archive{Files: map[string][]byte{}}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		if n := cleanName(h.Name); n != "" && !strings.HasPrefix(n, ".phar/") {
			a.Files[n] = b
		}
	}
	return a, nil
}

func readDir(dir string) (*Archive, error) {
	a := &Archive{Files: map[string][]byte{}}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		a.Files[filepath.ToSlash(rel)] = b
		return nil
	})
	return a, err
}

func cleanName(n string) string {
	n = strings.ReplaceAll(n, "\\", "/")
	n = path.Clean("/" + n)
	return strings.TrimPrefix(n, "/")
}

type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *reader) u32() uint32 {
	b := r.bytes(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *reader) u16() uint16 {
	b := r.bytes(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
