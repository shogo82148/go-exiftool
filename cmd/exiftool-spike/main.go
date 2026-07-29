// Command exiftool-spike is a feasibility spike: it proves that ExifTool (a
// pure-Perl program) runs unmodified on github.com/goccy/go-perl (Perl 5.42.2
// transpiled from wasm to pure Go) and extracts image metadata.
//
// Usage:
//
//	# get the ExifTool source (its lib/ tree) once:
//	git clone --depth 1 https://github.com/exiftool/exiftool /tmp/exiftool
//
//	go run ./cmd/exiftool-spike /tmp/exiftool/lib /tmp/exiftool/t/images/Canon.jpg
//
// It prints the extracted tag/value pairs, ExifTool's version, and timings.
//
// Findings (go-perl v0.1.0):
//   - Works with zero-config perl.NewInterpreter(perl.Config{}): the embedded
//     Perl stdlib is auto-extracted and the host "/" is visible to the guest,
//     so ExifTool's lib/ and the image are referenced by real host paths.
//   - The perl.Config{FS: ...} / perl.NewStdlibMemFS() path is BROKEN in
//     v0.1.0 (perl_new returns 0), even though it is the README example.
package main

import (
	"fmt"
	"os"
	"time"

	perl "github.com/goccy/go-perl"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: exiftool-spike <exiftool-lib-dir> <image-path>")
		os.Exit(2)
	}
	libDir, imgPath := os.Args[1], os.Args[2]

	start := time.Now()

	// Zero config: host "/" is visible to the guest and the embedded stdlib is
	// auto-extracted. (Config.FS / NewStdlibMemFS is broken in go-perl v0.1.0.)
	i, err := perl.NewInterpreter(perl.Config{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewInterpreter:", err)
		os.Exit(1)
	}
	defer i.Close()
	fmt.Printf("interpreter booted in %s\n", time.Since(start))

	// Load ExifTool from the host lib/ tree and extract via its public API.
	script := fmt.Sprintf(`
use lib '%s';
use Image::ExifTool qw(:Public);
my $et = Image::ExifTool->new;
my $info = $et->ImageInfo('%s');
if (my $err = $et->GetValue('Error')) { die "ExifTool error: $err\n"; }
for my $tag (sort keys %%$info) {
    my $val = $info->{$tag};
    $val = ref($val) eq 'ARRAY' ? join(', ', @$val) : $val;
    printf "%%-34s: %%s\n", $tag, $val;
}
"OK ExifTool v" . $Image::ExifTool::VERSION;
`, libDir, imgPath)

	r, err := i.Eval(script)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Eval (transport):", err)
		os.Exit(1)
	}
	fmt.Printf("eval done in %s\n\n", time.Since(start))

	if r.Stdout != "" {
		fmt.Println("--- extracted metadata ---")
		fmt.Print(r.Stdout)
		fmt.Println("--------------------------")
	}
	if !r.Ok {
		fmt.Printf("\nEVAL FAILED: %s\nstderr: %s\n", r.Error, r.Stderr)
		os.Exit(1)
	}
	fmt.Printf("\nRESULT: %s\n", r.Result)
}
