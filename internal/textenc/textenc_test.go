package textenc

import (
	"strings"
	"testing"
)

func TestUTF8(t *testing.T) {
	text, enc, err := Decode([]byte("Name\nZoë Testperson\n"))
	if err != nil || enc != UTF8 || text != "Name\nZoë Testperson\n" {
		t.Fatalf("got %q %q %v", text, enc, err)
	}
}

func TestUTF8BOMStripped(t *testing.T) {
	text, enc, err := Decode([]byte("\xEF\xBB\xBFDate,Amount\n"))
	if err != nil || enc != UTF8BOM || text != "Date,Amount\n" {
		t.Fatalf("got %q %q %v", text, enc, err)
	}
}

func TestWindows1252(t *testing.T) {
	// "Renée – café €5" in Windows-1252.
	b := []byte("Ren\xE9e \x96 caf\xE9 \x805")
	text, enc, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if enc != Windows1252 || text != "Renée – café €5" {
		t.Fatalf("got %q %q", text, enc)
	}
}

func TestWindows1252UnusedByteRefused(t *testing.T) {
	// 0x81 is not used in Windows-1252, and the bytes are not UTF-8.
	if _, _, err := Decode([]byte("abc\x81def")); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestUTF16(t *testing.T) {
	le := []byte{0xFF, 0xFE, 'H', 0, 'i', 0, 0xE9, 0}
	text, enc, err := Decode(le)
	if err != nil || enc != UTF16LE || text != "Hié" {
		t.Fatalf("LE: got %q %q %v", text, enc, err)
	}
	be := []byte{0xFE, 0xFF, 0, 'H', 0, 'i'}
	text, enc, err = Decode(be)
	if err != nil || enc != UTF16BE || text != "Hi" {
		t.Fatalf("BE: got %q %q %v", text, enc, err)
	}
	if _, _, err := Decode([]byte{0xFF, 0xFE, 'H'}); err == nil {
		t.Fatal("odd length: want a refusal")
	}
	if _, _, err := Decode([]byte{0xFF, 0xFE, 0x00, 0xD8, 'a', 0}); err == nil {
		t.Fatal("lone surrogate: want a refusal")
	}
}

func TestBinaryRefused(t *testing.T) {
	for name, b := range map[string][]byte{
		"pdf": []byte("%PDF-1.7\n..."),
		"zip": []byte("PK\x03\x04rest"),
		"nul": []byte("a,b\n1,\x002\n"),
		"png": []byte("\x89PNG\r\n"),
		"xls": []byte("\xD0\xCF\x11\xE0rest"),
	} {
		_, _, err := Decode(b)
		if err == nil {
			t.Errorf("%s: want a refusal", name)
		} else if strings.Contains(err.Error(), "rest") {
			t.Errorf("%s: message echoes file content", name)
		}
	}
}
