# phar2go

**Convert PocketMine-MP plugins (`.phar`) into Go plugins for [PocketMine-go](https://github.com/PMGopher/pocketmine-go).**

Give it a `.phar`, get back a Go plugin module that compiles into the PocketMine-go server:
classes become Go types, event listeners, commands, tasks, configs and forms are mapped to
PocketMine-go's API, and the plugin's `plugin.yml` and `resources/` are kept.

```text
phar2go MyPlugin.phar
✓ MyPlugin v1.2.0 -> MyPlugin-go
  14 classes, 96 methods from 14 PHP files; module github.com/author/myplugin
```

## Download

Get the latest build for your system from the **[Releases](https://github.com/PMGopher/phar2go/releases)** page:

| System | File |
|---|---|
| Windows | `phar2go-<version>-windows-amd64.zip` |
| Linux | `phar2go-<version>-linux-amd64.tar.gz` (`arm64` for ARM) |
| macOS | `phar2go-<version>-darwin-arm64.tar.gz` (Apple silicon) or `darwin-amd64` (Intel) |

It is a single program with no dependencies. To **build** the converted plugin into a server you
need [Go 1.26+](https://go.dev/dl/) and a clone of
[pocketmine-go](https://github.com/PMGopher/pocketmine-go), like for any PocketMine-go plugin.

## Usage

### The easy way: straight into your server

```bash
phar2go -server path/to/pocketmine-go MyPlugin.phar
```

This converts the plugin into `pocketmine-go/plugins-go/<name>`, adds it to the server's `go.mod`
and `cmd/pocketmine-go/plugins.go`, and builds the server. Start the server as usual: the plugin
loads like any other.

On Windows (PowerShell):

```powershell
.\phar2go.exe -server C:\path\to\pocketmine-go MyPlugin.phar
```

You can also drag a `.phar` onto `phar2go.exe`: it writes `MyPlugin-go` next to it.

### Only convert

```bash
phar2go MyPlugin.phar                 # writes MyPlugin-go/
phar2go -o out/MyPlugin MyPlugin.phar # choose the folder
phar2go a.phar b.phar c.phar          # several plugins at once
phar2go path/to/PluginSourceFolder    # a plugin folder (with plugin.yml) works too
```

| Option | What it does |
|---|---|
| `-o <folder>` | Output folder (default `<name>-go` next to the input) |
| `-server <pocketmine-go>` | Install the plugin into a pocketmine-go clone and build the server |
| `-check <pocketmine-go>` | Build and vet the converted plugin against a pocketmine-go clone |
| `-module <path>` | Go module path of the plugin (default `github.com/<author>/<name>`) |
| `-force` | Overwrite the output folder |
| `-version` | Print the version |

### What you get

```text
MyPlugin-go/
├── go.mod
├── plugin.yml          the original, with main and api updated for pocketmine-go
├── plugin.go           registers the plugin with the server
├── plugin_test.go      starts a server and checks that the plugin loads
├── resources/          the plugin's default files, unchanged
├── README.md           how to add it to a server
├── CONVERSION.md       what was converted, and what needs a manual look
└── internal/
    ├── myplugin/       the converted code: one Go file per PHP file
    └── phpx/           a small runtime for PHP arrays, loose comparisons and PHP functions
```

A converted plugin is its own Go module (see its `go.mod`): add it to the server with
`phar2go -server`, or as its README explains. Importing it by folder path doesn't work.

## What is converted

- Classes, interfaces, traits, enums, abstract classes and inheritance (with virtual dispatch),
  constructors with promoted properties, static members and constants
- The plugin's main class (`onLoad`, `onEnable`, `onDisable`, `onCommand`), listeners with
  `@priority`/`@handleCancelled`/`@notHandler`, commands extending `Command`, tasks, async tasks,
  closures and arrow functions, forms (`pocketmine\form\Form`, FormAPI and other form libraries)
- Calls to the PocketMine-MP API, mapped to PocketMine-go's Go API using an index of PocketMine-go
  built into phar2go (method names, constructors, constants, registries like `VanillaItems`)
- PHP semantics: arrays (ordered maps with value semantics), loose comparisons, string
  conversion, `try`/`catch`/`finally`, `match`, `static` variables, by-reference parameters
- Around 350 PHP functions (strings, arrays, math, JSON, YAML, regular expressions with PCRE
  lookarounds, files, dates)
- Virions bundled in the phar are converted with the plugin

Code that has no equivalent is marked `TODO(phar2go)` and listed in `CONVERSION.md`. It still
compiles, and throws an error only if it runs. Typical cases: PocketMine-MP internals that
PocketMine-go doesn't have (raw network packets, NBT tags), PHP extensions (SQLite3, mysqli,
threads), generators (`yield`), `eval` and reflection.

## Building from source

```bash
git clone https://github.com/PMGopher/phar2go
cd phar2go
go build .
```

The API index (`internal/api/index.json.gz`) is generated from a pocketmine-go checkout. To update
it for a newer PocketMine-go:

```bash
go run ./cmd/genindex -pm ../pocketmine-go -pmmp ../PocketMine-MP
```

`-pmmp` (optional) adds the values of PocketMine-MP class constants that PocketMine-go doesn't have.

Tests: `go test ./...` (set `PHAR2GO_PM=../pocketmine-go` to also build and load a converted
plugin in a real server).
