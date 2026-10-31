package dataset

// The mature MIME/charset reader is test-only: the shipped binary must not
// link its network packages. This oracle compares full decoded fields and body
// in order, preserving case, amounts, punctuation and currency. Only whitespace
// is normalized, because the text view adds its own line breaks.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

func readMail(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "mail", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type oracleText struct {
	body  string
	plain bool
}

// oracleRender follows the same documented text-view policy using the mature
// reader's MIME boundaries, header decoding and charset conversion. HTMLToText
// is shared intentionally; its markup policy has separate expected-text tests.
func oracleRender(b []byte) (string, error) {
	ent, err := message.Read(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	body, err := oracleEntity(ent, 0)
	if err != nil {
		return "", err
	}
	return oracleFields(ent, body.body)
}

func oracleFields(ent *message.Entity, body string) (string, error) {
	var rendered strings.Builder
	for _, name := range []string{"From", "To", "Cc", "Date", "Subject"} {
		fields := ent.Header.FieldsByKey(name)
		for fields.Next() {
			v, err := fields.Text()
			if err != nil {
				return "", err
			}
			values := []string{v}
			if name == "To" || name == "Cc" {
				values = splitAddresses(v)
			}
			for _, value := range values {
				fmt.Fprintf(&rendered, "%s: %s\n", name, value)
			}
		}
	}
	rendered.WriteString("\n" + body)
	return rendered.String(), nil
}

func oracleEntity(ent *message.Entity, depth int) (oracleText, error) {
	if depth > MaxDepth {
		return oracleText{}, fmt.Errorf("oracle depth limit")
	}
	// The entire entity is skipped, including multipart attachments. Test the
	// disposition before opening either the multipart reader or the body.
	if ent.Header.Get("Content-Disposition") != "" {
		disposition, _, err := ent.Header.ContentDisposition()
		if err != nil {
			return oracleText{}, err
		}
		if strings.EqualFold(disposition, "attachment") {
			return oracleText{}, nil
		}
	}
	mediaType, _, err := ent.Header.ContentType()
	if err != nil {
		return oracleText{}, err
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := ent.MultipartReader()
		if reader == nil {
			return oracleText{}, fmt.Errorf("oracle multipart reader missing")
		}
		defer func() { _ = reader.Close() }() // cleanup if a child part fails
		finish := func(value oracleText) (oracleText, error) {
			if err := reader.Close(); err != nil {
				return oracleText{}, err
			}
			return value, nil
		}
		var children []oracleText
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return oracleText{}, err
			}
			child, err := oracleEntity(part, depth+1)
			if err != nil {
				return oracleText{}, err
			}
			if strings.TrimSpace(child.body) != "" {
				children = append(children, child)
			}
		}
		if mediaType == "multipart/alternative" {
			for _, child := range children {
				if child.plain {
					return finish(child)
				}
			}
			if len(children) > 0 {
				return finish(children[0])
			}
			return finish(oracleText{})
		}
		var texts []string
		result := oracleText{}
		for _, child := range children {
			texts = append(texts, child.body)
			result.plain = result.plain || child.plain
		}
		result.body = strings.Join(texts, "\n\n")
		return finish(result)
	}
	if mediaType != "message/rfc822" && mediaType != "text/plain" && mediaType != "text/html" {
		return oracleText{}, nil
	}
	body, err := io.ReadAll(ent.Body)
	if err != nil {
		return oracleText{}, err
	}
	switch mediaType {
	case "message/rfc822":
		inner, err := message.Read(bytes.NewReader(body))
		if err != nil {
			return oracleText{}, err
		}
		result, err := oracleEntity(inner, depth+1)
		if err != nil {
			return oracleText{}, err
		}
		rendered, err := oracleFields(inner, result.body)
		if err != nil {
			return oracleText{}, err
		}
		result.body = "--- Forwarded message ---\n" + rendered
		return result, nil
	case "text/html":
		return oracleText{body: HTMLToText(string(body))}, nil
	default:
		return oracleText{body: string(body), plain: true}, nil
	}
}

func assertOracleAgrees(t *testing.T, b []byte) {
	t.Helper()
	want, err := oracleRender(b)
	if err != nil {
		t.Fatalf("oracle: %v", err)
	}
	got, _, err := RenderEmail(b)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(want), " ") {
		t.Fatalf("full decoded text differs (only whitespace is normalized):\nwant %q\n got %q", want, got)
	}
}

func TestOracleAgreesOnFullDecodedText(t *testing.T) {
	for _, name := range []string{
		"simple.eml", "alternative.eml", "base64.eml", "attachment.eml", "forwarded.eml", "koi8r.eml", "html-only.eml",
	} {
		t.Run(name, func(t *testing.T) {
			assertOracleAgrees(t, readMail(t, name))
		})
	}
}

func TestOraclePreservesCurrencyAndIndependentBodies(t *testing.T) {
	for name, b := range map[string][]byte{
		"latin9": []byte("From: =?ISO-8859-15?Q?Zelda_=A4?= <z@example.com>\n" +
			"Subject: =?ISO-8859-15?Q?Invoice_=A4100.00?=\nContent-Type: text/plain; charset=iso-8859-15\n\nAmount due: \xa4100.00.\n"),
		"mixed-and-forwarded-html": []byte("From: sender@example.com\nSubject: invoice 123.45\nContent-Type: multipart/mixed; boundary=OUTER\n\n" +
			"--OUTER\nContent-Type: text/plain; charset=utf-8\n\nPlain note: 123.45.\n" +
			"--OUTER\nContent-Type: text/html; charset=utf-8\n\n<p>Separate invoice: €345.67.</p>\n" +
			"--OUTER\nContent-Type: message/rfc822\n\nFrom: Zelda Quorn <zelda@example.com>\nSubject: payment\nContent-Type: text/html; charset=utf-8\n\n<p>Forwarded invoice: €456.78.</p>\n" +
			"--OUTER--\n"),
		"multipart-attachment": []byte("From: sender@example.com\nContent-Type: multipart/mixed; boundary=OUTER\n\n" +
			"--OUTER\nContent-Type: text/plain\n\nVisible €123.45.\n--OUTER\n" +
			"Content-Type: multipart/mixed; boundary=INNER\nContent-Disposition: attachment\n\n" +
			"--INNER\nContent-Type: text/plain\n\nATTACHMENT SECRET Zelda Quorn.\n--INNER--\n--OUTER--\n"),
	} {
		t.Run(name, func(t *testing.T) { assertOracleAgrees(t, b) })
	}
}

func TestOraclePropagatesNestedDecodeFailures(t *testing.T) {
	for _, leaf := range []string{
		"Content-Type: text/plain; charset=ZeldaQuorn\n\nSecret body.\n",
		"Content-Type: text/plain\nContent-Transfer-Encoding: base64\n\n!!!!\n",
		"Content-Type: text/plain\nSubject: =?ZeldaQuorn?Q?secret?=\n\nBody.\n",
	} {
		// The third fixture's bad field belongs to a forwarded message, so it
		// is rendered as a field instead of being an unused part-level header.
		if strings.Contains(leaf, "Subject:") {
			leaf = "Content-Type: message/rfc822\n\nFrom: inner@example.com\n" + leaf
		}
		b := []byte("From: sender@example.com\nContent-Type: multipart/mixed; boundary=OUTER\n\n--OUTER\n" + leaf + "--OUTER--\n")
		if got, err := oracleRender(b); err == nil {
			t.Fatalf("oracle swallowed nested decode failure: %q", got)
		}
		if got, _, err := RenderEmail(b); err == nil || got != "" {
			t.Fatalf("production reader accepted nested decode failure: %q, %v", got, err)
		}
	}
}
