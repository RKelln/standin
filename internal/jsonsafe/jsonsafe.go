// Package jsonsafe checks JSON without accepting duplicate names, trailing
// content, or broken Unicode. Errors never contain data from the input.
package jsonsafe

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"
)

var errInvalid = errors.New("the JSON is damaged or has names listed twice")

// Validate checks exactly one JSON value, including all nested objects.
func Validate(b []byte) error {
	if !utf8.Valid(b) || !validSurrogates(b) {
		return errInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return errInvalid
		}
		t, err := d.Token()
		if err != nil {
			return errInvalid
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				t, err := d.Token()
				key, ok := t.(string)
				if err != nil || !ok || seen[key] {
					return errInvalid
				}
				seen[key] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			t, err := d.Token()
			if err != nil || t != json.Delim('}') {
				return errInvalid
			}
		case json.Delim('['):
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			t, err := d.Token()
			if err != nil || t != json.Delim(']') {
				return errInvalid
			}
		case json.Delim('}'), json.Delim(']'):
			return errInvalid
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return errInvalid
	}
	return nil
}

// encoding/json replaces lone UTF-16 surrogates instead of refusing them.
// Inspect escapes first so that the checker does not silently change a name.
func validSurrogates(b []byte) bool {
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		r, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if r >= 0xdc00 && r <= 0xdfff {
			return false
		}
		if r < 0xd800 || r > 0xdbff {
			continue
		}
		if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
