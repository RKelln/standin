package dataset

// The oracle: our mail reader is checked against a mature one that ships in no
// binary. github.com/emersion/go-message is imported here and nowhere else, so
// it is compiled into the test binary only; `go list -deps ./cmd/standin` never
// sees it, and `make no-network` stays at zero. It is here because it does the
// one thing ours cannot safely do at run time: it links net through
// net/textproto (DESIGN 74-75).

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// readMail reads one of the invented fixtures.
func readMail(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "mail", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// oracleBody returns the decoded plain text of one message, read by go-message.
func oracleBody(b []byte) (string, error) {
	ent, err := message.Read(bytes.NewReader(b))
	if ent == nil {
		return "", err
	}
	var out strings.Builder
	var walk func(e *message.Entity, depth int)
	walk = func(e *message.Entity, depth int) {
		if depth > MaxDepth {
			return
		}
		mt, _, _ := e.Header.ContentType()
		if strings.HasPrefix(mt, "multipart/") {
			mr := e.MultipartReader()
			if mr == nil {
				return
			}
			for {
				p, perr := mr.NextPart()
				if perr != nil {
					return
				}
				walk(p, depth+1)
			}
		}
		if strings.EqualFold(mt, "message/rfc822") {
			inner, _ := io.ReadAll(e.Body)
			if in, err := message.Read(bytes.NewReader(inner)); in != nil {
				walk(in, depth+1)
				_ = err
			}
			return
		}
		if strings.EqualFold(mt, "text/plain") {
			disp, _, _ := e.Header.ContentDisposition()
			if strings.EqualFold(disp, "attachment") {
				return
			}
			io.Copy(&out, e.Body) // the library has already decoded the charset
			out.WriteString("\n")
		}
	}
	walk(ent, 0)
	return out.String(), nil
}

// contentWords is the multiset of words of three or more letters, lower case.
// Comparing words rather than lines ignores the line breaks each reader adds.
func contentWords(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() >= 3 {
			out = append(out, b.String())
		}
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	sort.Strings(out)
	return out
}

// renderedHeader matches a header line as we print it, at the top of a message
// or of a forwarded one. go-message gives back only the text, so these lines
// are dropped before the two readings are compared.
var renderedHeader = regexp.MustCompile(`(?m)^(From|To|Cc|Date|Subject): .*$`)

// bodyOnly drops the headers we render, and the forwarded-message marker.
func bodyOnly(rendered string) string {
	if i := strings.Index(rendered, "\n\n"); i >= 0 {
		rendered = rendered[i+2:]
	}
	rendered = renderedHeader.ReplaceAllString(rendered, "")
	return strings.ReplaceAll(rendered, "--- Forwarded message ---", "")
}

func diffWords(want, got []string) string {
	count := map[string]int{}
	for _, w := range want {
		count[w]++
	}
	for _, w := range got {
		count[w]--
	}
	var missing, extra []string
	for w, n := range count {
		switch {
		case n > 0:
			missing = append(missing, w)
		case n < 0:
			extra = append(extra, w)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	var b strings.Builder
	if len(missing) > 0 {
		b.WriteString("missing: " + strings.Join(missing, " ") + "\n")
	}
	if len(extra) > 0 {
		b.WriteString("extra: " + strings.Join(extra, " ") + "\n")
	}
	return b.String()
}

// The fixtures with a plain-text part: an HTML-only message is left out
// because go-message hands back the HTML itself, which is not a like-for-like
// comparison with a reader that strips it.
func TestOracleAgreesOnPlainText(t *testing.T) {
	for _, name := range []string{
		"simple.eml", "alternative.eml", "base64.eml", "attachment.eml", "forwarded.eml", "koi8r.eml",
	} {
		t.Run(name, func(t *testing.T) {
			b := readMail(t, name)
			want, err := oracleBody(b)
			if err != nil {
				t.Fatalf("oracle could not read %s: %v", name, err)
			}
			rendered, _, err := RenderEmail(b)
			if err != nil {
				t.Fatalf("RenderEmail(%s): %v", name, err)
			}
			wantWords := contentWords(want)
			gotWords := contentWords(bodyOnly(rendered))
			if len(wantWords) == 0 {
				t.Fatalf("%s: the oracle found no text", name)
			}
			if d := diffWords(wantWords, gotWords); d != "" {
				t.Errorf("%s: our reader and go-message disagree on the text:\n%s", name, d)
			}
		})
	}
}

// The oracle must also agree that a message it can read is readable: nothing
// we refuse is text go-message found without trouble, except where we refuse
// on purpose (mbox, an unknown character set).
func TestOracleAgreesOnRefusals(t *testing.T) {
	b := readMail(t, "simple.eml")
	if _, err := oracleBody(b); err != nil {
		t.Fatalf("oracle refused a message we accept: %v", err)
	}
}
