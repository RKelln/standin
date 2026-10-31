package dataset

// Email: one message per .eml file, turned into the plain-text form the rest of
// Standin already understands (fields at the top, free text below).
//
// The scrub copy is a readable view, not the original message file: the MIME
// structure, the HTML and any attachment are not kept. Restore writes the same
// view back with the real values in place. The original .eml is never changed.
// This keeps the standard-library-only, offline, plain-text contract: no MIME
// reader from net/mail or mime/multipart, because both link the net package.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/quotedprintable"
	"regexp"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/textenc"
)

// EmailInfo describes how a message file was turned into text. The shape file
// reports it so the person knows what is missing.
type EmailInfo struct {
	Parts       int  // MIME parts read
	Attachments int  // parts not read because they are attachments or not text
	FromHTML    bool // the text came from an HTML part
}

// MaxParts and MaxDepth bound a hostile or broken message.
const (
	MaxParts = 200
	MaxDepth = 6
)

// RenderEmail turns one RFC 822 message into plain text: the fields a reader
// needs (From, To, Cc, Date, Subject) and the readable body.
func RenderEmail(b []byte) (string, *EmailInfo, error) {
	if what, ok := textenc.Sniff(b); ok {
		return "", nil, fail.New("not-text", "This file is %s, not a message. Save the message as .eml from your mail program, then try again.", what)
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return "", nil, fail.New("not-text", "This message holds binary data. Save it as .eml from your mail program, then try again.")
	}
	head, body, err := splitMessage(string(b))
	if err != nil {
		return "", nil, err
	}
	hdr, err := parseHeaders(head)
	if err != nil {
		return "", nil, err
	}
	if !looksLikeMessage(hdr) {
		return "", nil, fail.New("email-structure", "This does not look like one email message. It needs header lines like From: and Subject:. If it holds several messages, save each one as its own .eml file.")
	}
	info := &EmailInfo{}
	w := &messageWalker{info: info}
	if err := w.walk(hdr, body, 0); err != nil {
		return "", nil, err
	}
	if len(w.plain) == 0 && len(w.html) > 0 {
		info.FromHTML = true
		for _, h := range w.html {
			w.plain = append(w.plain, HTMLToText(h))
		}
	}
	text := strings.Join(w.plain, "\n\n")
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if strings.TrimSpace(text) == "" {
		return "", nil, fail.New("email-empty", "This message has no text I can read. It may hold only an attachment or an image. Save the text you need in a .txt file, or remove private details by hand.")
	}
	return renderMessage(hdr, text), info, nil
}

// mboxFrom matches the separator line an mbox file puts before each message:
// "From sender@example.com Wed Sep  2 10:14:00 2026".
var mboxFrom = regexp.MustCompile(`^From \S+ (?:Mon|Tue|Wed|Thu|Fri|Sat|Sun) (?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [ 0-9]\d \d\d:\d\d:\d\d \d{4}`)

// splitMessage separates the header block from the body at the first blank
// line. An mbox file holds several messages, so it is refused rather than
// read as one.
func splitMessage(s string) (head, body string, err error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	firstLine := s
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		firstLine = s[:i]
	}
	if mboxFrom.MatchString(firstLine) {
		return "", "", fail.New("email-structure", "This looks like an mbox file, which holds several messages in one file. Save each message as its own .eml file, or paste the messages into a .md file, then try again.")
	}
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[:i], s[i+2:], nil
	}
	if strings.TrimSpace(s) == "" {
		return "", "", fail.New("empty", "This file is empty.")
	}
	return "", "", fail.New("email-structure", "This does not look like an email message: it has no blank line between the header lines and the text. Save it as .eml from your mail program, then try again.")
}

// parseHeaders reads Name: value lines, joining folded continuation lines.
// Any line without a colon is refused: that is how mbox and saved web pages
// fail, and guessing would put text in the wrong place.
func parseHeaders(head string) (map[string][]string, error) {
	hdr := map[string][]string{}
	var name string
	for _, line := range strings.Split(head, "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if name == "" {
				return nil, fail.New("email-structure", "This does not look like an email message: it starts with a continuation line. Save it as .eml from your mail program, then try again.")
			}
			v := hdr[name]
			v[len(v)-1] += " " + strings.TrimSpace(line)
			hdr[name] = v
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			return nil, fail.New("email-structure", "This does not look like an email message: a header line has no name and colon. Save it as .eml from your mail program, then try again.")
		}
		name = strings.ToLower(strings.TrimSpace(line[:i]))
		if !headerName(name) {
			return nil, fail.New("email-structure", "This does not look like an email message: a header line has a name I do not expect. Save it as .eml from your mail program, then try again.")
		}
		hdr[name] = append(hdr[name], strings.TrimSpace(line[i+1:]))
	}
	return hdr, nil
}

// headerName allows the printable ASCII a header name may use (RFC 5322).
func headerName(s string) bool {
	if s == "" || len(s) > 78 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && strings.IndexByte("-._", c) < 0 {
			return false
		}
	}
	return true
}

func looksLikeMessage(hdr map[string][]string) bool {
	for _, h := range []string{"from", "to", "cc", "subject", "date"} {
		if len(hdr[h]) > 0 {
			return true
		}
	}
	return false
}

// messageWalker collects the readable text of a message and counts what it
// could not read.
type messageWalker struct {
	info  *EmailInfo
	plain []string
	html  []string
	parts int
}

func (w *messageWalker) walk(hdr map[string][]string, body string, depth int) error {
	if depth > MaxDepth {
		return fail.New("email-structure", "This message nests parts too deeply for me to follow safely. Save the message you need as a .txt file, then try again.")
	}
	w.parts++
	if w.parts > MaxParts {
		return fail.New("email-structure", "This message has too many parts for me to follow safely. Save the part you need as a .txt file, then try again.")
	}
	w.info.Parts++
	mediaType, params, err := contentType(hdr)
	if err != nil {
		return err
	}
	cte := first(hdr, "content-transfer-encoding")
	isAttachment := strings.HasPrefix(strings.ToLower(strings.TrimSpace(first(hdr, "content-disposition"))), "attachment")

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return fail.New("email-structure", "This message says it has several parts, but does not say how they are separated. Save the message as .eml again, then try again.")
		}
		parts, err := splitMultipart(body, boundary)
		if err != nil {
			return err
		}
		for _, part := range parts {
			ph, pb, err := splitMessage(part)
			if err != nil {
				// A part that is only text (no header lines) is read as it is.
				ph, pb = "", part
			}
			phdr, err := parseHeaders(ph)
			if err != nil {
				return err
			}
			if len(phdr) == 0 {
				// No header lines: read it as a plain text part.
				phdr = map[string][]string{"content-type": {"text/plain"}}
				pb = part
			}
			if err := w.walk(phdr, pb, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if isAttachment {
		w.info.Attachments++
		return nil
	}
	switch mediaType {
	case "text/plain":
		text, err := decodePart(body, cte, params["charset"])
		if err != nil {
			return err
		}
		w.plain = append(w.plain, text)
	case "text/html":
		text, err := decodePart(body, cte, params["charset"])
		if err != nil {
			return err
		}
		w.html = append(w.html, text)
	case "message/rfc822":
		inner, innerBody, ok := forwarded(body)
		if !ok {
			w.info.Attachments++
			return nil
		}
		// Keep the forwarded message's own fields, so the person and the AI
		// still see who wrote it.
		var b strings.Builder
		b.WriteString("--- Forwarded message ---\n")
		for _, h := range []string{"From", "To", "Date", "Subject"} {
			for _, v := range headerValues(inner, h) {
				fmt.Fprintf(&b, "%s: %s\n", h, v)
			}
		}
		b.WriteString("\n")
		w.plain = append(w.plain, b.String())
		return w.walk(inner, innerBody, depth+1)
	default:
		w.info.Attachments++
	}
	return nil
}

// forwarded reads the message inside a message/rfc822 part. A part that is
// not a readable message is counted as an attachment, not refused: the rest of
// the mail is still worth reading.
func forwarded(body string) (map[string][]string, string, bool) {
	head, rest, ok := strings.Cut(body, "\n\n")
	if !ok {
		return nil, "", false
	}
	hdr, err := parseHeaders(head)
	if err != nil || len(hdr) == 0 {
		return nil, "", false
	}
	return hdr, rest, true
}

// contentType reads the part's Content-Type, defaulting to text/plain.
func contentType(hdr map[string][]string) (string, map[string]string, error) {
	raw := strings.TrimSpace(first(hdr, "content-type"))
	if raw == "" {
		return "text/plain", map[string]string{}, nil
	}
	mediaType, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", nil, fail.New("email-structure", "This message has a header I cannot read (%s). Save it as .eml again from your mail program, then try again.", fail.Reason(err))
	}
	return strings.ToLower(mediaType), params, nil
}

// splitMultipart divides a body on its boundary lines. The delimiter is
// "--boundary"; the closing one is "--boundary--".
func splitMultipart(body, boundary string) ([]string, error) {
	delim := "--" + boundary
	var parts, cur []string
	inPart := false
	closed := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(t, delim) {
			rest := strings.TrimSpace(t[len(delim):])
			if rest == "" || rest == "--" {
				if inPart {
					parts = append(parts, strings.Join(cur, "\n"))
				}
				cur = nil
				inPart = rest == ""
				if rest == "--" {
					closed = true
					break
				}
				continue
			}
		}
		if inPart {
			cur = append(cur, line)
		}
	}
	if inPart {
		parts = append(parts, strings.Join(cur, "\n"))
	}
	if len(parts) == 0 || !closed {
		return nil, fail.New("email-structure", "This message says it has several parts, but they are not laid out the way I expect. Save it as .eml again from your mail program, then try again.")
	}
	return parts, nil
}

// decodePart undoes the transfer encoding and the character set of one part.
func decodePart(body, cte, charset string) (string, error) {
	raw, err := decodeTransfer(body, cte)
	if err != nil {
		return "", err
	}
	return textenc.DecodeCharset(raw, charset)
}

func decodeTransfer(s, cte string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "", "7bit", "8bit", "binary":
		return []byte(s), nil
	case "quoted-printable":
		b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(s)))
		if err != nil {
			return nil, fail.New("email-encoding", "Part of this message is in a form I cannot undo (quoted-printable). Save the message as .eml again, then try again.")
		}
		return b, nil
	case "base64":
		clean := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\n', '\r', '\t':
				return -1
			}
			return r
		}, s)
		b, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "="))
		}
		if err != nil {
			return nil, fail.New("email-encoding", "Part of this message is base64 but does not decode. Save the message as .eml again, then try again.")
		}
		return b, nil
	}
	return nil, fail.New("email-encoding", "Part of this message uses a form I cannot undo (%s). Save the message as .eml again, then try again.", strings.TrimSpace(cte))
}

// renderMessage writes the fields a reader needs, then the body.
func renderMessage(hdr map[string][]string, body string) string {
	var b strings.Builder
	for _, name := range []string{"From", "To", "Cc", "Date", "Subject"} {
		for _, v := range headerValues(hdr, name) {
			fmt.Fprintf(&b, "%s: %s\n", name, v)
		}
	}
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

// headerValues decodes one header, one line per address for To, Cc and Bcc.
func headerValues(hdr map[string][]string, name string) []string {
	raw := hdr[strings.ToLower(name)]
	if len(raw) == 0 {
		return nil
	}
	var out []string
	for _, v := range raw {
		v = decodeHeader(v)
		if name == "To" || name == "Cc" || name == "Bcc" {
			out = append(out, splitAddresses(v)...)
			continue
		}
		out = append(out, oneLine(v))
	}
	return out
}

// decodeHeader undoes RFC 2047 encoded words (=?UTF-8?Q?...?=) and folds the
// value onto one line. An encoded word Standin cannot decode is a refusal: the
// name inside it would otherwise stay hidden.
func decodeHeader(v string) string {
	dec := &mime.WordDecoder{CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		b, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		s, err := textenc.DecodeCharset(b, charset)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(s), nil
	}}
	out, err := dec.DecodeHeader(v)
	if err != nil {
		// Keep the raw text: an undecoded word still looks like a name to the
		// word check less often, but it is not hidden behind an encoder.
		return oneLine(v)
	}
	return oneLine(out)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

// splitAddresses splits a To or Cc value on commas that are not inside quotes
// or angle brackets.
func splitAddresses(v string) []string {
	var out []string
	depth, quoted := 0, false
	start := 0
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '"':
			quoted = !quoted
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 && !quoted {
				out = append(out, strings.TrimSpace(v[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(v[start:]))
	var keep []string
	for _, a := range out {
		if a != "" {
			keep = append(keep, a)
		}
	}
	return keep
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// Go's regexp has no backreferences, so each element is spelled out.
	htmlHidden = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>|<head\b[^>]*>.*?</head\s*>|<!doctype[^>]*>`)
	htmlBreak  = regexp.MustCompile(`(?i)<\s*(?:br|hr)\s*/?\s*>`)
	htmlBlock  = regexp.MustCompile(`(?i)</?\s*(?:p|div|tr|li|h[1-6]|table|blockquote|pre|section|article|header|footer|ul|ol|dl|dd|dt|form)\b[^>]*>`)
	htmlCell   = regexp.MustCompile(`(?i)</?\s*(?:td|th)\b[^>]*>`)
	htmlTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	blankLines = regexp.MustCompile(`\n{3,}`)
	trailSpace = regexp.MustCompile(`[ \t]+\n`)
)

// HTMLToText keeps the words of an HTML part and drops the markup, the styles
// and the links. It is a reading aid, not an HTML parser: the plain-text part
// is always preferred when a message has one.
func HTMLToText(s string) string {
	s = htmlComment.ReplaceAllString(s, "")
	s = htmlHidden.ReplaceAllString(s, "")
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlBlock.ReplaceAllString(s, "\n")
	s = htmlCell.ReplaceAllString(s, " ")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = trailSpace.ReplaceAllString(s, "")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func first(hdr map[string][]string, name string) string {
	if v := hdr[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}
