package exiftool_test

import (
	"os"
	"testing"

	exiftool "github.com/shogo82148/go-exiftool"
)

// newET builds an interpreter shared by the subtests and closes it on cleanup.
func newET(t *testing.T) *exiftool.ExifTool {
	t.Helper()
	et, err := exiftool.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { et.Close() })
	return et
}

func TestExtractCanon(t *testing.T) {
	et := newET(t)
	f, err := et.Extract("testdata/Canon.jpg")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(f.Tags) == 0 {
		t.Fatal("no tags extracted")
	}

	make_, ok := f.Get("Make")
	if !ok {
		t.Fatal("Make tag missing")
	}
	if make_.Print != "Canon" {
		t.Errorf("Make = %q, want %q", make_.Print, "Canon")
	}

	// FileType comes from the File group.
	if ft, ok := f.GetGroup("File", "FileType"); !ok || ft.Print != "JPEG" {
		t.Errorf("File:FileType = %q ok=%v, want JPEG", ft.Print, ok)
	}

	// A composite tag proves ExifTool's derived tags ran.
	if _, ok := f.Get("ImageSize"); !ok {
		t.Error("Composite ImageSize missing")
	}
}

func TestExtractGPSValueVsPrint(t *testing.T) {
	et := newET(t)
	f, err := et.Extract("testdata/GPS.jpg")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	lat, ok := f.Get("GPSLatitude")
	if !ok {
		t.Fatal("GPSLatitude missing")
	}
	// Print is human-readable; Value is the machine form. They should differ
	// for a coordinate, and both be non-empty.
	if lat.Print == "" {
		t.Error("GPSLatitude Print empty")
	}
	if lat.Value == nil {
		t.Error("GPSLatitude Value nil")
	}
	t.Logf("GPSLatitude value=%v print=%q", lat.Value, lat.Print)
}

func TestExtractFormats(t *testing.T) {
	et := newET(t)
	cases := map[string]string{
		"testdata/PNG.png":      "image/png",
		"testdata/ExifTool.tif": "image/tiff",
		"testdata/Canon.jpg":    "image/jpeg",
	}
	for path, wantMIME := range cases {
		f, err := et.Extract(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		mt, ok := f.Get("MIMEType")
		if !ok || mt.Print != wantMIME {
			t.Errorf("%s: MIMEType = %q ok=%v, want %q", path, mt.Print, ok, wantMIME)
		}
	}
}

func TestExtractBytes(t *testing.T) {
	et := newET(t)
	data, err := os.ReadFile("testdata/Canon.jpg")
	if err != nil {
		t.Fatal(err)
	}
	f, err := et.ExtractBytes("photo.jpg", data)
	if err != nil {
		t.Fatalf("ExtractBytes: %v", err)
	}
	if f.Path != "photo.jpg" {
		t.Errorf("Path = %q, want photo.jpg", f.Path)
	}
	if m, ok := f.Get("Make"); !ok || m.Print != "Canon" {
		t.Errorf("Make = %q ok=%v, want Canon", m.Print, ok)
	}
}

func TestGroupsFilter(t *testing.T) {
	et := newET(t)
	f, err := et.Extract("testdata/GPS.jpg", exiftool.Groups("GPS"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(f.Tags) == 0 {
		t.Fatal("no GPS tags")
	}
	for _, tag := range f.Tags {
		if tag.Group != "GPS" {
			t.Errorf("non-GPS tag leaked through filter: %s:%s", tag.Group, tag.Name)
		}
	}
}

func TestExtractErrorOnEmpty(t *testing.T) {
	et := newET(t)
	// ExifTool reports an Error for an empty file; we surface it as *ExtractError.
	// (Note: arbitrary text is not an error — ExifTool recognizes it as TXT.)
	_, err := et.ExtractBytes("empty.jpg", []byte{})
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	ee, ok := err.(*exiftool.ExtractError)
	if !ok {
		t.Fatalf("error type = %T, want *exiftool.ExtractError (%v)", err, err)
	}
	if ee.Path != "empty.jpg" {
		t.Errorf("ExtractError.Path = %q, want empty.jpg", ee.Path)
	}
}

func TestClosedExtract(t *testing.T) {
	et, err := exiftool.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	et.Close()
	if _, err := et.Extract("testdata/Canon.jpg"); err == nil {
		t.Error("expected error extracting on closed ExifTool")
	}
}
