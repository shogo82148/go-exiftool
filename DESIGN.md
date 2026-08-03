# go-exiftool — API Design

Design for a pure-Go ExifTool wrapper built on
[`goccy/go-perl`](https://github.com/goccy/go-perl). Feasibility is confirmed
(see [README](README.md)); this document fixes the public API before
implementation.

## Verified foundations

These behaviours were verified against go-perl v0.1.0 and drive the design:

1. **Zero-config interpreter only.** `perl.NewInterpreter(perl.Config{})` works;
   the host `/` is visible to the guest, so ExifTool's `lib/` and target files
   are referenced by real host paths. The `Config.FS` / `NewStdlibMemFS` path is
   broken in v0.1.0 — do not use it.
2. **Interpreter reuse works.** Boot once (~420 ms), `require Image::ExifTool`
   once, then run many `Extract` calls (~90 ms each) on the same interpreter —
   verified across repeated `Eval`s. Persistent state survives between calls.
3. **JSON bridge.** `JSON::PP` 4.16 ships in the embedded stdlib, so the Perl
   side encodes a record array and Go unmarshals it — no fragile text parsing.
4. **Both value forms are available per tag.** `GetGroup($tag, 1)` gives the
   specific group (`GPS`, `IFD0`, `Canon`, `Composite`, ...), `GetValue($tag,
   'PrintConv')` the human-readable form, `GetValue($tag, 'ValueConv')` the
   machine form. JSON preserves numbers as numbers.

## Package layout

```
exiftool/            # root package: New, ExifTool, Extract, Fields, Tag, options
  exiftool.go        # public API
  bridge.go          # Perl source templates + JSON<->Go marshalling
  embed.go           # //go:embed of the trimmed ExifTool lib (build-tagged)
  lib/               # generated: trimmed ExifTool lib/ tree (code only, no pod)
cmd/exiftool-spike/  # existing feasibility spike (kept)
```

The ExifTool `lib/` is **embedded by default** via `go:embed` (decision:
self-contained single binary, no `exiftool` install). `New()` extracts it to a
temp dir once per process and prepends it to `@INC`. `WithLibDir(path)` overrides
the embed to point at an external tree (smaller builds, or a pinned ExifTool
version).

## Public API

```go
package exiftool

// New boots one Perl interpreter and loads Image::ExifTool (~420 ms). Reuse the
// returned *ExifTool across many Extract calls (see Concurrency).
func New(opts ...Option) (*ExifTool, error)

// Close finalizes the interpreter and removes the extracted lib temp dir.
func (e *ExifTool) Close() error

// Extract reads metadata from one file on the host filesystem.
func (e *ExifTool) Extract(path string, opts ...ExtractOption) (*Fields, error)

// ExtractBytes reads metadata from in-memory data. The bytes are written to a
// temp file (the guest sees the host FS) whose extension is taken from name so
// ExifTool can pick the right reader; name also becomes Fields.Path.
func (e *ExifTool) ExtractBytes(name string, data []byte, opts ...ExtractOption) (*Fields, error)

// Fields is the metadata extracted from one file.
type Fields struct {
    Path     string   // file path (or name for ExtractBytes)
    Tags     []Tag    // every extracted tag, in ExifTool order
    Warnings []string // ExifTool minor warnings (non-fatal)
}

// Tag is one metadata value.
type Tag struct {
    Group string // family-1 group: EXIF, GPS, IFD0, ExifIFD, Canon, Composite, File, ...
    Name  string // tag name, e.g. "Model", "GPSLatitude"
    Value any    // machine value (ValueConv): string | float64 | bool | []any | map[string]any
    Print string // human-readable value (PrintConv), e.g. `54 deg 59' 22.80" N`
}

// Consumption helpers.
func (f *Fields) Get(name string) (Tag, bool)          // first tag with Name (case-insensitive)
func (f *Fields) GetGroup(group, name string) (Tag, bool)
func (f *Fields) Map() map[string]any                  // Name -> Value (last wins on dupes)
func (f *Fields) PrintMap() map[string]string          // Name -> Print
func (f *Fields) GroupMap() map[string]map[string]any  // Group -> Name -> Value
```

Both value forms are kept on every `Tag` (decision: keep both). `Value` is for
computation, `Print` for display. When a tag has no distinct conversion the two
are equal.

## Options

Interpreter-level (`Option`, passed to `New`):

| Option | Maps to | Default |
|---|---|---|
| `WithLibDir(path)` | override embedded lib | embedded |
| `WithStdlibDir(path)` | `perl.Config.StdlibDir` | embedded stdlib |
| `WithCharset(cs)` | `Options(Charset => ...)` | `UTF8` |
| `WithDateFormat(fmt)` | `Options(DateFormat => ...)` | ExifTool default |

Per-call (`ExtractOption`, passed to `Extract`):

| Option | Maps to | Default |
|---|---|---|
| `Tags(names...)` | `RequestTags` / tag filter | all tags |
| `Groups(names...)` | family-0/1 group filter | all groups |
| `Composite(bool)` | `Options(Composite => ...)` | on |
| `Duplicates(bool)` | `Options(Duplicates => ...)` | on |
| `Binary(bool)` | include binary tag data (base64) | off |
| `ExtractEmbedded(bool)` | `Options(ExtractEmbedded => ...)` | off |

Binary tags are omitted by default; with `Binary(true)` they are returned
base64-encoded (as `exiftool -b -j` does).

## Perl bridge

`New()` runs once:

```perl
unshift @INC, '<libdir>';
require Image::ExifTool;
require JSON::PP;
```

`Extract()` runs a template like:

```perl
my $et = Image::ExifTool->new;
$et->Options(Duplicates => 1, Composite => 1, Charset => 'UTF8', %opts);
$et->ExtractInfo('<path>');
my @tags;
for my $t ($et->GetFoundTags) {
    push @tags, {
        group => $et->GetGroup($t, 1),
        name  => Image::ExifTool::GetTagName($t),
        value => scalarize($et->GetValue($t, 'ValueConv')),
        print => scalarize($et->GetValue($t, 'PrintConv')),
    };
}
my @warn = $et->GetValue('Warning') ... ;
print JSON::PP->new->utf8->canonical->encode({ tags => \@tags, warnings => \@warn });
```

Go reads `EvalResult.Stdout`, `json.Unmarshal`s into `Fields`.

## Error handling

- **Go `error`**: interpreter/transport failure (`Eval` transport error), or an
  ExifTool `Error` tag (unreadable/unsupported file). The Go error wraps
  ExifTool's message.
- **`Fields.Warnings`**: ExifTool minor warnings (e.g. `[minor] ...`). Non-fatal;
  `Fields` is still returned.
- A Perl `die` inside a call surfaces as a Go error, never a panic.

## Concurrency

`*ExifTool` owns one interpreter. **No wrapper-level mutex is added** — go-perl's
`Module.invoke` already guards every `Eval` with a per-interpreter
`sync.Mutex`, and each `Extract` is a single self-contained `Eval` (build `$et`,
`ExtractInfo`, encode JSON), so concurrent `Extract` calls on one `*ExifTool`
serialize safely at the go-perl layer with no shared-state interleaving. Adding
our own lock would only duplicate that.

This gives no real parallelism on a single instance (calls queue). For parallel
extraction, create multiple `*ExifTool` instances (each ~one interpreter's
memory). A managed `exiftool.Pool` of N interpreters is a documented follow-up,
not in v1.

## Out of scope for v1

- Writing/editing metadata (`WriteInfo`) — read-only for now.
- `io.Reader` streaming without a temp file (ExifTool needs a seekable source).
- The `exiftool.Pool` type.

## Example

```go
et, err := exiftool.New()
if err != nil { log.Fatal(err) }
defer et.Close()

f, err := et.Extract("photo.jpg")
if err != nil { log.Fatal(err) }

if lat, ok := f.Get("GPSLatitude"); ok {
    fmt.Println(lat.Print) // 54 deg 59' 22.80" N
    fmt.Println(lat.Value) // 54.9896666...
}
for _, t := range f.Tags {
    fmt.Printf("%-12s %-24s %v\n", t.Group, t.Name, t.Print)
}
```
