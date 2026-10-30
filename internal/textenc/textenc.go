// Package textenc turns file bytes into text, and refuses files that are not text.
//
// Rules: strip a UTF-8 byte order mark; decode UTF-16 when it starts with a byte
// order mark; use UTF-8 when the bytes are valid UTF-8; otherwise use
// Windows-1252 only if every byte fits it. Anything else is refused.
package textenc

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/fail"
)

// Names of the encodings Decode reports.
const (
	UTF8        = "UTF-8"
	UTF8BOM     = "UTF-8 with a byte order mark"
	UTF16LE     = "UTF-16 (little end first)"
	UTF16BE     = "UTF-16 (big end first)"
	Windows1252 = "Windows-1252"
)

// win1252 maps bytes 0x80 to 0x9F. Zero means the byte is not used in Windows-1252.
var win1252 = [32]rune{
	0x20AC, 0, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017D, 0,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0, 0x017E, 0x0178,
}

// Binary file starts that are never text, whatever the file name says.
var magic = []struct {
	prefix string
	what   string
}{
	{"%PDF", "a PDF"},
	{"PK\x03\x04", "a zipped file (like Excel .xlsx or Word .docx)"},
	{"\xD0\xCF\x11\xE0", "an old Office file (like .xls or .doc)"},
	{"\x89PNG", "an image"},
	{"\xFF\xD8\xFF", "an image"},
	{"GIF8", "an image"},
}

// Sniff returns a description if the bytes look like a known binary file.
func Sniff(b []byte) (string, bool) {
	for _, m := range magic {
		if bytes.HasPrefix(b, []byte(m.prefix)) {
			return m.what, true
		}
	}
	return "", false
}

// Decode returns the text and the name of the encoding used.
func Decode(b []byte) (string, string, error) {
	if what, ok := Sniff(b); ok {
		return "", "", fail.New("not-text", "This file is %s, not text.", what)
	}
	enc := UTF8
	var text string
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		enc = UTF8BOM
		b = b[3:]
		if !utf8.Valid(b) {
			return "", "", fail.New("bad-encoding", "This file says it is UTF-8 but some bytes are not UTF-8.")
		}
		text = string(b)
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		t, err := decode16(b[2:], false)
		if err != nil {
			return "", "", err
		}
		enc, text = UTF16LE, t
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		t, err := decode16(b[2:], true)
		if err != nil {
			return "", "", err
		}
		enc, text = UTF16BE, t
	case utf8.Valid(b):
		text = string(b)
	default:
		t, ok := decode1252(b)
		if !ok {
			return "", "", fail.New("bad-encoding", "I cannot tell how the text in this file is stored. Save it again as CSV UTF-8, then try again.")
		}
		enc, text = Windows1252, t
	}
	if hasControl(text) {
		return "", "", fail.New("not-text", "This file has bytes that are not text. It may not be a text file.")
	}
	return text, enc, nil
}

func decode16(b []byte, bigEndian bool) (string, error) {
	if len(b)%2 != 0 {
		return "", fail.New("bad-encoding", "This UTF-16 file is cut short (odd number of bytes).")
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if bigEndian {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	// Check surrogate pairs, so a broken file is refused, not silently changed.
	for i := 0; i < len(u); i++ {
		switch {
		case u[i] >= 0xD800 && u[i] < 0xDC00:
			if i+1 >= len(u) || u[i+1] < 0xDC00 || u[i+1] >= 0xE000 {
				return "", fail.New("bad-encoding", "This UTF-16 file has broken characters.")
			}
			i++
		case u[i] >= 0xDC00 && u[i] < 0xE000:
			return "", fail.New("bad-encoding", "This UTF-16 file has broken characters.")
		}
	}
	text := string(utf16.Decode(u))
	text = trimBOM(text)
	return text, nil
}

func trimBOM(s string) string {
	if len(s) >= 3 && s[:3] == "\xEF\xBB\xBF" {
		return s[3:]
	}
	return s
}

func decode1252(b []byte) (string, bool) {
	var sb []rune
	for _, c := range b {
		switch {
		case c < 0x80:
			sb = append(sb, rune(c))
		case c < 0xA0:
			r := win1252[c-0x80]
			if r == 0 {
				return "", false
			}
			sb = append(sb, r)
		default:
			sb = append(sb, rune(c))
		}
	}
	return string(sb), true
}

// hasControl reports control characters other than tab, line feed and carriage return.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return true
		}
	}
	return false
}
