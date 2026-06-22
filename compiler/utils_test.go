package compiler

import (
	"strings"
	"testing"

	"github.com/gopherjs/gopherjs/internal/srctesting"
)

func Test_encodeString(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantUtf8  string
		wantUtf16 string
	}{
		{
			name:      `empty`,
			in:        ``,
			wantUtf8:  `""`,
			wantUtf16: `""`,
		},
		{
			name:      `ascii`,
			in:        `hello`,
			wantUtf8:  `"hello"`,
			wantUtf16: `"hello"`,
		},
		{
			name:      `escape backslash`,
			in:        `a\b`,
			wantUtf8:  `"a\\b"`,
			wantUtf16: `"a\\b"`,
		},
		{
			name:      `escape double quote`,
			in:        `a"b`,
			wantUtf8:  `"a\"b"`,
			wantUtf16: `"a\"b"`,
		},
		{
			name:      `escape control runes`,
			in:        "\b\f\n\r\t\v",
			wantUtf8:  `"\b\f\n\r\t\v"`,
			wantUtf16: `"\b\f\n\r\t\v"`,
		},
		{
			name:      `escape low non-printable`,
			in:        "\x00\x01\x1f",
			wantUtf8:  `"\x00\x01\x1F"`,
			wantUtf16: `"\x00\x01\x1F"`,
		},
		{
			name:      `escape DEL (with é for non-ASCII path)`,
			in:        "\u00e9\x7f",
			wantUtf8:  `"\xC3\xA9\x7F"`,
			wantUtf16: `"\u00E9\u007F"`,
		},
		{
			name:      `printable boundary`,
			in:        " ~",
			wantUtf8:  `" ~"`,
			wantUtf16: `" ~"`,
		},
		{
			name:      `UTF-8 BMP bytes (é)`,
			in:        "h\xc3\xa9llo",
			wantUtf8:  `"h\xC3\xA9llo"`,
			wantUtf16: `"h\u00E9llo"`,
		},
		{
			name:      `UTF-8 CJK bytes`,
			in:        "日本語",
			wantUtf8:  `"\xE6\x97\xA5\xE6\x9C\xAC\xE8\xAA\x9E"`,
			wantUtf16: `"\u65E5\u672C\u8A9E"`,
		},
		{
			name:      `UTF-8 supplementary plane bytes and UTF-16 surrogate pair (😀)`,
			in:        "\U0001F600",
			wantUtf8:  `"\xF0\x9F\x98\x80"`,
			wantUtf16: `"\uD83D\uDE00"`,
		},
		{
			name:      `control char in non-ASCII for UTF-16 path stays escaped as \n`,
			in:        "\u00e9\n",
			wantUtf8:  `"\xC3\xA9\n"`,
			wantUtf16: `"\u00E9\n"`,
		},
		{
			name:      `UTF-16 high BMP edge (U+FFFF)`,
			in:        "\uffff",
			wantUtf8:  `"\xEF\xBF\xBF"`,
			wantUtf16: `"\uFFFF"`,
		},
		{
			name:      `UTF-16 low supplementary edge (U+10000)`,
			in:        "\U00010000",
			wantUtf8:  `"\xF0\x90\x80\x80"`,
			wantUtf16: `"\uD800\uDC00"`,
		},
		{
			name:      `UTF-16 high supplementary edge (U+10FFFF)`,
			in:        "\U0010FFFF",
			wantUtf8:  `"\xF4\x8F\xBF\xBF"`,
			wantUtf16: `"\uDBFF\uDFFF"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeString(tt.in); got != tt.wantUtf8 {
				t.Errorf("UTF-8: encodeString(%q):\n\tgot:  %q\n\twant: %q", tt.in, got, tt.wantUtf8)
			}
			if got := preexternalizeString(tt.in); got != tt.wantUtf16 {
				t.Errorf("UTF-16: preexternalizeString(%q):\n\tgot:  %q\n\twant: %q", tt.in, got, tt.wantUtf16)
			}
		})
	}
}

func TestExternalize_LiteralVsRuntime(t *testing.T) {
	const src = `
		package main

		import "github.com/gopherjs/gopherjs/js"

		func setStuff(obj *js.Object, dyn string) {
			obj.Set("ascii", "hello")
			obj.Set("unicode", "h` + "\u00e9" + `llo")
			obj.Set("emoji", "` + "\U0001F600" + `")
			obj.Set("dynamic", dyn)
			obj.Set("concat", "pre-" + dyn)
		}

		func main() {
			setStuff(nil, "")
		}
	`
	srcFiles := []srctesting.Source{{Name: `main.go`, Contents: []byte(src)}}
	out := compile(t, srcFiles, false)

	mustContain := []struct {
		name string
		good string
		bad  string
	}{
		{
			name: "ASCII literal pre-externalized as plain JS string",
			good: `"hello"`,
		},
		{
			name: "BMP literal pre-externalized as UTF-16 escapes",
			good: `"h\u00E9llo"`,
		},
		{
			name: "Supplementary literal pre-externalized as surrogate pair",
			good: `"\uD83D\uDE00"`,
		},
		{
			name: "Plain string variable falls back to runtime $externalize",
			good: `$externalize(dyn, $String)`,
		},
		{
			name: "Computed (non-constant) string falls back to runtime $externalize",
			good: `$externalize("pre-" + dyn, $String)`,
		},
		{
			name: "ASCII literal must not be wrapped in $externalize",
			bad:  `$externalize("hello"`,
		},
		{
			name: "BMP literal must not be wrapped in $externalize",
			bad:  `$externalize("h\u00E9llo"`,
		},
		{
			name: "Supplementary literal must not be wrapped in $externalize",
			bad:  `$externalize("\uD83D\uDE00"`,
		},
	}
	for _, c := range mustContain {
		t.Run(c.name, func(t *testing.T) {
			if len(c.good) > 0 && !strings.Contains(out, c.good) {
				t.Errorf("compiled output missing %q\n--- output ---\n%s", c.good, out)
			}
			if len(c.bad) > 0 && strings.Contains(out, c.bad) {
				t.Errorf("compiled output unexpectedly contains %q\n--- output ---\n%s", c.bad, out)
			}
		})
	}
}
