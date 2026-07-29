# go-exiftool

Run [ExifTool](https://exiftool.org/) on top of
[`goccy/go-perl`](https://github.com/goccy/go-perl) — a pure-Go build of Perl
5.42.2 (transpiled from WebAssembly, no cgo, no external `perl`). The goal is a
single static Go binary that extracts image metadata without shelling out to an
installed `exiftool`.

> **Status: feasibility spike.** This repo currently contains only a
> verification program, not the finished library. Feasibility is confirmed —
> see below.

## Feasibility findings (2026-07-29)

ExifTool is pure Perl with core-only dependencies (Perl 5.004+), and go-perl
ships the Perl standard library plus static XS extensions. That combination
runs ExifTool **unmodified**:

- ExifTool **13.59** boots and extracts full metadata (Exif / MakerNote / GPS /
  IPTC) from JPEG, PNG, and TIFF — output matches native `exiftool`.
- Interpreter boot: ~420–480 ms. Per-image extraction: ~90 ms. The interpreter
  keeps state across `Eval` calls, so reuse one instance to pay boot cost once.
- Pure Go: `CGO_ENABLED=0` friendly, single self-contained binary.

### go-perl v0.1.0 gotcha

The `perl.Config{FS: ...}` / `perl.NewStdlibMemFS()` filesystem-backend path is
**broken in v0.1.0** (`perl_new returned 0`), even though it is the README
example. The working approach is zero-config `perl.NewInterpreter(perl.Config{})`:
the embedded stdlib is auto-extracted and the host `/` is visible to the guest,
so ExifTool's `lib/` and the target image are referenced by their real host
paths.

## Running the spike

```bash
# fetch the ExifTool source (its lib/ tree and test images) once
git clone --depth 1 https://github.com/exiftool/exiftool /tmp/exiftool

# extract metadata from an image
go run ./cmd/exiftool-spike /tmp/exiftool/lib /tmp/exiftool/t/images/Canon.jpg
```

## Planned library API

```go
et, err := exiftool.New()        // boot interpreter + load embedded ExifTool lib
defer et.Close()
meta, err := et.Extract("photo.jpg")   // map of tag -> value
```

Open design questions: embedding ExifTool's `lib/` via `go:embed`; output shape
(JSON round-trip vs. tag map); input sources (path vs. `[]byte`/`io.Reader`).

## License

The finished library will embed ExifTool's Perl modules and go-perl's Perl
standard library, both of which carry Perl's dual **Artistic / GPL** license and
require the corresponding notices. The Go code here is otherwise the author's.
