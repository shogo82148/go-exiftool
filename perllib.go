package exiftool

// Embedded ExifTool Perl library.
//
// perllib.zip is a trimmed copy of the ExifTool distribution's lib/ tree — only
// the .pm/.pl code, no pod/html/data files — so a `go build` yields a single
// binary that runs ExifTool without a system install. It is extracted once per
// process into a temp directory and prepended to the interpreter's @INC.
//
// Regenerate with `make perllib` (see Makefile); keep ExifToolVersion in sync.

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ExifToolVersion is the ExifTool release vendored in perllib.zip.
const ExifToolVersion = "13.59"

//go:embed perllib.zip
var perllibZip []byte

var (
	perllibOnce sync.Once
	perllibPath string
	perllibErr  error
)

// extractLib unpacks the embedded ExifTool library into a temp directory (once
// per process) and returns its path, suitable for prepending to @INC. The
// result is cached, so repeated New calls share one extraction.
func extractLib() (string, error) {
	perllibOnce.Do(func() {
		perllibPath, perllibErr = extractZipTo(perllibZip, "go-exiftool-lib-")
	})
	return perllibPath, perllibErr
}

// extractZipTo unpacks an embedded zip under a fresh temp dir (named with
// prefix) and returns that dir.
func extractZipTo(data []byte, prefix string) (string, error) {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open embedded zip: %w", err)
	}
	root := filepath.Clean(dir)
	for _, f := range zr.File {
		// Guard against zip-slip: the cleaned target must stay under dir.
		target := filepath.Join(dir, f.Name)
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return "", fmt.Errorf("illegal path in embedded zip: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		if err := extractZipFile(f, target); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
