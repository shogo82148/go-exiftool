package exiftool

// Perl bridge: the Perl source run inside the go-perl interpreter and the
// JSON decoding of its output back into Go values.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The Perl templates below contain literal '%' (hash sigils) and are filled by
// token replacement, never fmt.Sprintf, so those percents need no escaping.

// bootScript loads ExifTool and JSON::PP once, after @INC has been pointed at
// the extracted library. Kept minimal so any load failure surfaces from New.
// __LIBDIR__ is replaced with a Perl-quoted path.
const bootScript = `
unshift @INC, __LIBDIR__;
require Image::ExifTool;
require JSON::PP;
"go-exiftool boot v" . $Image::ExifTool::VERSION;
`

// buildBootScript fills bootScript with the (Perl-quoted) library directory.
func buildBootScript(libDir string) string {
	return strings.Replace(bootScript, "__LIBDIR__", perlQuote(libDir), 1)
}

// extractScript is the per-file template. It builds a fresh ExifTool object,
// extracts every found tag as {group,name,value,print}, routes ExifTool's own
// Error/Warning tags aside, and prints one JSON document the Go side decodes.
//
// _norm collapses Perl references to JSON-native values: array refs to arrays,
// hash refs (structured XMP) to objects, blessed/other refs to their string
// form, plain scalars unchanged (JSON::PP preserves numeric scalars as numbers).
const extractScript = `
{
    my $et = Image::ExifTool->new;
    $et->Options(__OPTIONS__);
    $et->ExtractInfo(__PATH__);
    my $norm;
    $norm = sub {
        my $v = shift;
        return undef unless defined $v;
        my $r = ref $v;
        if ($r eq 'ARRAY')  { return [ map { $norm->($_) } @$v ]; }
        if ($r eq 'HASH')   { return { map { $_ => $norm->($v->{$_}) } keys %$v }; }
        if ($r eq 'SCALAR') { return $$v; }
        if ($r)             { return "$v"; }
        return $v;
    };
    my (@tags, @warnings, $error);
    for my $t ($et->GetFoundTags()) {
        my $name = Image::ExifTool::GetTagName($t);
        my $g0   = $et->GetGroup($t, 0);
        if ($g0 eq 'ExifTool' && $name eq 'Error')   { $error = "" . $et->GetValue($t, 'ValueConv'); next; }
        if ($g0 eq 'ExifTool' && $name eq 'Warning') { push @warnings, "" . $et->GetValue($t, 'ValueConv'); next; }
        my $p = $et->GetValue($t, 'PrintConv');
        $p = defined $p ? (ref $p eq 'ARRAY' ? join(', ', @$p) : "$p") : undef;
        push @tags, {
            group => $et->GetGroup($t, 1),
            name  => $name,
            value => $norm->($et->GetValue($t, 'ValueConv')),
            print => $p,
        };
    }
    print JSON::PP->new->utf8->allow_nonref->canonical->encode(
        { tags => \@tags, warnings => \@warnings, error => $error });
}
`

// buildExtractScript fills extractScript with the ExifTool Options() argument
// list and the Perl-quoted host path.
func buildExtractScript(optionsList, hostPath string) string {
	s := strings.Replace(extractScript, "__OPTIONS__", optionsList, 1)
	return strings.Replace(s, "__PATH__", perlQuote(hostPath), 1)
}

// rawResult is the JSON shape emitted by extractScript.
type rawResult struct {
	Tags []struct {
		Group string          `json:"group"`
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
		Print *string         `json:"print"`
	} `json:"tags"`
	Warnings []string `json:"warnings"`
	Error    *string  `json:"error"`
}

// decodeResult turns the interpreter's stdout JSON into a *Fields.
func decodeResult(path, stdout string) (*Fields, error) {
	var raw rawResult
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, fmt.Errorf("decode ExifTool output: %w (output: %q)", err, truncate(stdout, 200))
	}
	if raw.Error != nil && *raw.Error != "" {
		return nil, &ExtractError{Path: path, Message: *raw.Error}
	}
	f := &Fields{Path: path, Warnings: raw.Warnings}
	f.Tags = make([]Tag, 0, len(raw.Tags))
	for _, t := range raw.Tags {
		var val any
		if len(t.Value) > 0 {
			_ = json.Unmarshal(t.Value, &val) // best-effort; leaves nil on failure
		}
		var print string
		if t.Print != nil {
			print = *t.Print
		}
		f.Tags = append(f.Tags, Tag{Group: t.Group, Name: t.Name, Value: val, Print: print})
	}
	return f, nil
}

// perlQuote renders s as a Perl single-quoted string literal. Only backslash
// and the single quote are special inside '...'.
func perlQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
