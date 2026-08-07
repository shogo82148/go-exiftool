// Package exiftool runs ExifTool (a pure-Perl program) on top of
// github.com/goccy/go-perl — a pure-Go build of Perl — to extract image
// metadata without shelling out to an installed exiftool.
//
// A single interpreter boots in a few hundred milliseconds; reuse one
// *ExifTool across many Extract calls, which are fast by comparison:
//
//	et, err := exiftool.New()
//	if err != nil { log.Fatal(err) }
//	defer et.Close()
//
//	f, err := et.Extract("photo.jpg")
//	if err != nil { log.Fatal(err) }
//	if t, ok := f.Get("Model"); ok {
//		fmt.Println(t.Print)
//	}
//
// Each Tag carries both the machine value (Value, from ExifTool's ValueConv)
// and the human-readable form (Print, from PrintConv).
package exiftool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	perl "github.com/goccy/go-perl"
)

// ExifTool is one Perl interpreter with Image::ExifTool loaded. Reuse it across
// Extract calls. It is safe to call from multiple goroutines: the underlying
// go-perl interpreter serializes evaluations, so concurrent Extract calls queue
// rather than run in parallel. For real parallelism, create several instances.
type ExifTool struct {
	interp *perl.Interpreter
	cfg    config
	libDir string
}

type config struct {
	libDir     string
	stdlibDir  string
	charset    string
	dateFormat string
}

// Option configures New.
type Option func(*config)

// WithLibDir points ExifTool at an external lib/ tree instead of the embedded
// copy (smaller builds, or a pinned ExifTool version). The path must contain
// Image/ExifTool.pm.
func WithLibDir(path string) Option { return func(c *config) { c.libDir = path } }

// WithStdlibDir overrides the Perl standard library location (defaults to the
// stdlib embedded in go-perl).
func WithStdlibDir(path string) Option { return func(c *config) { c.stdlibDir = path } }

// WithCharset sets the character set ExifTool decodes tag values into
// (ExifTool's Charset option). Defaults to UTF8.
func WithCharset(cs string) Option { return func(c *config) { c.charset = cs } }

// WithDateFormat sets a strftime format for date/time tags (ExifTool's
// DateFormat option). Empty leaves ExifTool's default.
func WithDateFormat(f string) Option { return func(c *config) { c.dateFormat = f } }

// New boots a Perl interpreter, extracts the embedded ExifTool library (unless
// WithLibDir is given), and loads Image::ExifTool.
func New(opts ...Option) (*ExifTool, error) {
	cfg := config{charset: "UTF8"}
	for _, o := range opts {
		o(&cfg)
	}

	libDir := cfg.libDir
	if libDir == "" {
		dir, err := extractLib()
		if err != nil {
			return nil, fmt.Errorf("extract embedded ExifTool lib: %w", err)
		}
		libDir = dir
	} else {
		if _, err := os.Stat(filepath.Join(libDir, "Image", "ExifTool.pm")); err != nil {
			return nil, fmt.Errorf("WithLibDir %q: Image/ExifTool.pm not found: %w", libDir, err)
		}
	}

	interp, err := perl.NewInterpreter(perl.Config{StdlibDir: cfg.stdlibDir})
	if err != nil {
		return nil, fmt.Errorf("start Perl interpreter: %w", err)
	}

	et := &ExifTool{interp: interp, cfg: cfg, libDir: libDir}
	r, err := interp.Eval(buildBootScript(libDir))
	if err != nil {
		interp.Close()
		return nil, fmt.Errorf("load Image::ExifTool: %w", err)
	}
	if !r.Ok {
		interp.Close()
		return nil, fmt.Errorf("load Image::ExifTool: %s", strings.TrimSpace(r.Error))
	}
	return et, nil
}

// Close finalizes the interpreter. The ExifTool must not be used afterward.
func (e *ExifTool) Close() error {
	if e.interp == nil {
		return nil
	}
	err := e.interp.Close()
	e.interp = nil
	return err
}

// Extract reads metadata from a file on the host filesystem.
func (e *ExifTool) Extract(path string, opts ...ExtractOption) (*Fields, error) {
	if e.interp == nil {
		return nil, fmt.Errorf("exiftool: Extract on closed ExifTool")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	return e.extract(abs, path, opts)
}

// ExtractBytes reads metadata from in-memory data. The bytes are written to a
// temp file whose extension is taken from name (so ExifTool selects the right
// reader); name also becomes Fields.Path.
func (e *ExifTool) ExtractBytes(name string, data []byte, opts ...ExtractOption) (*Fields, error) {
	if e.interp == nil {
		return nil, fmt.Errorf("exiftool: ExtractBytes on closed ExifTool")
	}
	tmp, err := os.CreateTemp("", "go-exiftool-in-*"+filepath.Ext(name))
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	return e.extract(tmp.Name(), name, opts)
}

func (e *ExifTool) extract(hostPath, reportPath string, opts []ExtractOption) (*Fields, error) {
	eo := extractOptions{composite: true, duplicates: true}
	for _, o := range opts {
		o(&eo)
	}
	script := buildExtractScript(e.optionsList(eo), hostPath)
	r, err := e.interp.Eval(script)
	if err != nil {
		return nil, fmt.Errorf("exiftool: interpreter error: %w", err)
	}
	if !r.Ok {
		return nil, fmt.Errorf("exiftool: %s", strings.TrimSpace(r.Error))
	}
	f, err := decodeResult(reportPath, r.Stdout)
	if err != nil {
		return nil, err
	}
	if len(eo.groups) > 0 || len(eo.tags) > 0 {
		f.Tags = filterTags(f.Tags, eo)
	}
	return f, nil
}

// optionsList renders the ExifTool ->Options(...) argument list for one extract.
func (e *ExifTool) optionsList(eo extractOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Composite => %d, Duplicates => %d, Binary => %d",
		boolToInt(eo.composite), boolToInt(eo.duplicates), boolToInt(eo.binary))
	if e.cfg.charset != "" {
		fmt.Fprintf(&b, ", Charset => %s", perlQuote(e.cfg.charset))
	}
	if e.cfg.dateFormat != "" {
		fmt.Fprintf(&b, ", DateFormat => %s", perlQuote(e.cfg.dateFormat))
	}
	if eo.extractEmbedded {
		b.WriteString(", ExtractEmbedded => 1")
	}
	return b.String()
}

func filterTags(tags []Tag, eo extractOptions) []Tag {
	out := tags[:0:0]
	for _, t := range tags {
		if len(eo.groups) > 0 && !containsFold(eo.groups, t.Group) {
			continue
		}
		if len(eo.tags) > 0 && !containsFold(eo.tags, t.Name) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
