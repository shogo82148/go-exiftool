package exiftool

import (
	"fmt"
	"strings"
)

// Fields is the metadata extracted from one file.
type Fields struct {
	// Path is the file path passed to Extract (or the name given to
	// ExtractBytes).
	Path string
	// Tags holds every extracted tag, in ExifTool's found order. A file may
	// yield several tags with the same Name in different Groups.
	Tags []Tag
	// Warnings holds ExifTool's non-fatal warnings, if any.
	Warnings []string
}

// Tag is one metadata value.
type Tag struct {
	// Group is the specific (family-1) group: EXIF, GPS, IFD0, ExifIFD, Canon,
	// Composite, File, System, ...
	Group string
	// Name is the tag name, e.g. "Model" or "GPSLatitude".
	Name string
	// Value is the machine value (ExifTool's ValueConv), decoded from JSON:
	// string, float64, bool, []any, or map[string]any. It may be nil.
	Value any
	// Print is the human-readable value (ExifTool's PrintConv), e.g.
	// `54 deg 59' 22.80" N`.
	Print string
}

// Get returns the first tag whose Name matches name (case-insensitive).
func (f *Fields) Get(name string) (Tag, bool) {
	for _, t := range f.Tags {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Tag{}, false
}

// GetGroup returns the first tag matching both group and name
// (case-insensitive), disambiguating tags that share a Name.
func (f *Fields) GetGroup(group, name string) (Tag, bool) {
	for _, t := range f.Tags {
		if strings.EqualFold(t.Group, group) && strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Tag{}, false
}

// Map returns a Name -> Value map. When a name appears in multiple groups the
// last tag wins; use Tags or GetGroup to see every occurrence.
func (f *Fields) Map() map[string]any {
	m := make(map[string]any, len(f.Tags))
	for _, t := range f.Tags {
		m[t.Name] = t.Value
	}
	return m
}

// PrintMap returns a Name -> Print map. Last tag wins on duplicate names.
func (f *Fields) PrintMap() map[string]string {
	m := make(map[string]string, len(f.Tags))
	for _, t := range f.Tags {
		m[t.Name] = t.Print
	}
	return m
}

// GroupMap returns a Group -> (Name -> Value) map.
func (f *Fields) GroupMap() map[string]map[string]any {
	m := make(map[string]map[string]any)
	for _, t := range f.Tags {
		g := m[t.Group]
		if g == nil {
			g = make(map[string]any)
			m[t.Group] = g
		}
		g[t.Name] = t.Value
	}
	return m
}

// ExtractError reports an ExifTool-level failure to read a file (unsupported
// format, corrupt data, ...) as distinct from an interpreter/transport error.
type ExtractError struct {
	Path    string
	Message string
}

func (e *ExtractError) Error() string {
	return fmt.Sprintf("exiftool: %s: %s", e.Path, e.Message)
}

// extractOptions is the resolved per-call configuration.
type extractOptions struct {
	composite       bool
	duplicates      bool
	binary          bool
	extractEmbedded bool
	groups          []string
	tags            []string
}

// ExtractOption configures a single Extract call.
type ExtractOption func(*extractOptions)

// Composite toggles ExifTool's derived Composite tags (default on).
func Composite(on bool) ExtractOption { return func(o *extractOptions) { o.composite = on } }

// Duplicates toggles keeping duplicate tags from different groups (default on).
func Duplicates(on bool) ExtractOption { return func(o *extractOptions) { o.duplicates = on } }

// Binary includes binary tag data (base64-encoded) instead of a placeholder
// (default off).
func Binary(on bool) ExtractOption { return func(o *extractOptions) { o.binary = on } }

// ExtractEmbedded extracts metadata from embedded documents/thumbnails
// (ExifTool's ExtractEmbedded option, default off).
func ExtractEmbedded(on bool) ExtractOption {
	return func(o *extractOptions) { o.extractEmbedded = on }
}

// Groups limits the returned tags to the given family-1 groups
// (case-insensitive), e.g. Groups("GPS", "EXIF").
func Groups(names ...string) ExtractOption { return func(o *extractOptions) { o.groups = names } }

// Tags limits the returned tags to the given tag names (case-insensitive).
func Tags(names ...string) ExtractOption { return func(o *extractOptions) { o.tags = names } }
