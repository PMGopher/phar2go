// Command genindex regenerates the API index embedded in phar2go from a pocketmine-go checkout:
//
//	go run ./cmd/genindex -pm ../pocketmine-go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/PMGopher/phar2go/internal/api"
)

func main() {
	pm := flag.String("pm", "../pocketmine-go", "path to a pocketmine-go checkout")
	out := flag.String("o", "internal/api/index.json.gz", "output file")
	flag.Parse()
	idx, err := api.Generate(*pm, "./runtime/phpx")
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
