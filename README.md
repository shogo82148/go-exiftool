# go-exiftool

Run [ExifTool](https://exiftool.org/) on top of
[`goccy/go-perl`](https://github.com/goccy/go-perl) — a pure-Go build of Perl
5.42.2 (transpiled from WebAssembly, no cgo, no external `perl`). A single static
Go binary extracts image metadata without shelling out to an installed
`exiftool`: ExifTool's Perl library is embedded (`go:embed`) and runs inside an
in-process Perl interpreter.

> **Status: working (v1, read-only).** Extraction is implemented and tested.
> Metadata writing, an interpreter pool, and `io.Reader` streaming are not yet
> implemented. See [DESIGN.md](DESIGN.md).

Bundled ExifTool: **13.59**. Requires Go with `//go:embed`.

## Usage

```go
package main

import (
	"fmt"
	"log"

	exiftool "github.com/shogo82148/go-exiftool"
)

func main() {
	et, err := exiftool.New() // boots the interpreter, loads embedded ExifTool
	if err != nil {
		log.Fatal(err)
	}
	defer et.Close()

	f, err := et.Extract("photo.jpg")
	if err != nil {
		log.Fatal(err)
	}

	if t, ok := f.Get("Model"); ok {
		fmt.Println("Camera:", t.Print)
	}
	if lat, ok := f.Get("GPSLatitude"); ok {
		fmt.Println(lat.Print) // human-readable: 54 deg 59' 22.80" N
		fmt.Println(lat.Value) // machine value
	}
	for _, t := range f.Tags {
		fmt.Printf("%-10s %-24s %v\n", t.Group, t.Name, t.Print)
	}
}
```

Reuse one `*ExifTool` across many files — booting the interpreter costs a few
hundred milliseconds, each `Extract` is fast by comparison.

### API sketch

```go
func New(opts ...Option) (*ExifTool, error)
func (e *ExifTool) Extract(path string, opts ...ExtractOption) (*Fields, error)
func (e *ExifTool) ExtractBytes(name string, data []byte, opts ...ExtractOption) (*Fields, error)
func (e *ExifTool) Close() error

type Fields struct {
	Path     string
	Tags     []Tag    // every tag, in ExifTool order
	Warnings []string
}
type Tag struct {
	Group string // EXIF, GPS, IFD0, Canon, Composite, File, ...
	Name  string
	Value any    // machine value (ValueConv): string | float64 | bool | []any | map
	Print string // human-readable (PrintConv)
}
```

Helpers: `Fields.Get`, `Fields.GetGroup`, `Fields.Map`, `Fields.PrintMap`,
`Fields.GroupMap`. Options: `WithLibDir`, `WithCharset`, `WithDateFormat`;
per-call `Groups`, `Tags`, `Binary`, `Composite`, `Duplicates`, `ExtractEmbedded`.

Each `Tag` keeps both value forms: `Value` (ExifTool's ValueConv, for
computation) and `Print` (PrintConv, for display).

## Concurrency

`*ExifTool` owns one interpreter. Concurrent `Extract` calls are safe but
serialize (go-perl's `Module.invoke` locks per interpreter). For parallel
extraction, create multiple instances.

## How it works

`New` extracts the embedded ExifTool `lib/` (`perllib.zip`) to a temp dir, boots
a go-perl interpreter with zero config (host `/` visible to the guest), and
`require`s `Image::ExifTool`. Each `Extract` runs a small Perl program that calls
`ExtractInfo`, collects tags as `{group, name, value, print}`, and returns them
as JSON (via the bundled `JSON::PP`) for Go to decode.

### go-perl v0.1.0 note

The `perl.Config{FS: ...}` / `perl.NewStdlibMemFS()` filesystem-backend path is
**broken in v0.1.0** (`perl_new returned 0`), even though it is that project's
README example. This library uses zero-config `perl.NewInterpreter` instead and
references files by real host paths.

## Development

```bash
make test        # run the test suite
make perllib     # regenerate perllib.zip from a pinned ExifTool release
```

The feasibility spike is preserved under
[`cmd/exiftool-spike`](cmd/exiftool-spike).

## License

The Go code is the author's. `perllib.zip` embeds ExifTool's Perl modules and
go-perl embeds the Perl standard library; both carry Perl's dual **Artistic /
GPL** license, so a distributed binary should carry the corresponding notices.
Sample images under `testdata/` are ExifTool's own test fixtures.
