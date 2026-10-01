// Command genindex regenerates the API index embedded in phar2go from a pocketmine-go checkout:
//
//	go run ./cmd/genindex -pm ../pocketmine-go
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/PMGopher/phar2go/internal/api"
)

func main() {
	pm := flag.String("pm", "../pocketmine-go", "path to a pocketmine-go checkout")
	out := flag.String("o", "internal/api/index.json.gz", "output file")
	pmmp := flag.String("pmmp", "", "PocketMine-MP checkout: adds the literal values of its class constants")
	libs := flag.String("phplibs", "", "folders of PocketMine-MP libraries (BedrockProtocol, NBT, ...), comma separated")
	flag.Parse()
	idx, err := api.Generate(*pm, "./runtime/phpx")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *pmmp != "" {
		dirs := []string{*pmmp + "/src"}
		if *libs != "" {
			dirs = append(dirs, strings.Split(*libs, ",")...)
		}
		consts, defaults, err := api.GeneratePHPInfo(dirs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		idx.PHPConsts, idx.PHPDefaults = consts, defaults
		fmt.Printf("%d PocketMine-MP constants, %d methods with default arguments\n", len(idx.PHPConsts), len(idx.PHPDefaults))
	}
	if err := idx.Save(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d packages, %d PHP classes, %d PHP members\n", *out, len(idx.Packages), len(idx.PHPClasses), len(idx.PHPMembers))
}
