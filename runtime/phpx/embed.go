package phpx

import "embed"

// Source is this package's source code: phar2go copies it into every converted plugin.
//
//go:embed *.go
var Source embed.FS
