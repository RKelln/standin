package dataset

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/jsonsafe"
	"github.com/RKelln/standin/internal/scan"
)

// jnode is a JSON value that keeps key order.
type jnode struct {
	kind  byte // 'o' object, 'a' array, 's' scalar
	keys  []string
	vals  []*jnode // object values, or array elements
	raw   string   // scalar as JSON text
	str   string   // scalar as a value: string text, number text, true/false, "" for null
	isStr bool
}

const maxDepth = 64

func errJSON() error {
	return fail.New("bad-json", "This JSON file is not valid JSON, so I stopped. Check it in a JSON checker, then try again.")
}

func parseJSON(text string) (*Dataset, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	root, err := readNode(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fail.New("bad-json", "This JSON file has extra text after the end of the data, so I stopped.")
	}
	// The standard JSON decoder replaces lone surrogate escapes. Refuse
	// those inputs instead of silently changing a name before scrubbing it.
	if err := jsonsafe.Validate([]byte(text)); err != nil {
		return nil, errJSON()
	}
	d := &Dataset{root: root, colOf: map[string]int{}}
	add := func(path, v string, rec int) {
		i, ok := d.colOf[path]
		if !ok {
			i = len(d.Columns)
			d.colOf[path] = i
			d.Columns = append(d.Columns, path)
			d.Values = append(d.Values, nil)
			d.Recs = append(d.Recs, nil)
		}
		d.Values[i] = append(d.Values[i], v)
		d.Recs[i] = append(d.Recs[i], rec)
	}
	switch root.kind {
	case 'a':
		d.JSONShape = "array of records"
		for i, rec := range root.vals {
			if rec.kind != 'o' {
				return nil, fail.New("json-shape", "Record %d in this JSON list is not an object (a set of \"name\": value pairs). I only read a list of objects, or an object of objects.", i+1)
			}
			if err := flatten("", rec, func(p, v string) { add(p, v, i+1) }); err != nil {
				return nil, err
			}
		}
	case 'o':
		d.JSONShape = "object of records"
		for i, k := range root.keys {
			rec := root.vals[i]
			if rec.kind != 'o' {
				return nil, fail.New("json-shape", "Entry %d in this JSON object is not an object. I only read a list of objects, or an object of objects.", i+1)
			}
			add(KeyColumn, k, i+1)
			if err := flatten("", rec, func(p, v string) { add(p, v, i+1) }); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fail.New("json-shape", "This JSON file holds one plain value, not records. I only read a list of objects, or an object of objects.")
	}
	d.Rows = len(root.vals)
	if d.Rows == 0 {
		return nil, fail.New("empty", "This JSON file has no records.")
	}
	return d, nil
}

func readNode(dec *json.Decoder, depth int) (*jnode, error) {
	if depth > maxDepth {
		return nil, fail.New("json-shape", "This JSON file is nested too deep for me.")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, errJSON()
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			n := &jnode{kind: 'o'}
			seen := map[string]bool{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, errJSON()
				}
				k, ok := kt.(string)
				if !ok {
					return nil, errJSON()
				}
				// Validate the tree, not only its flattened scalar columns: a
				// key on an empty object or array still reaches the output.
				if hasHidden(k) {
					return nil, fail.New("hidden-char", "A JSON name (key) holds a hidden character. Remove it, then try again.")
				}
				if scan.HasStandin(k) {
					return nil, fail.New("standin-in-input", "A JSON name (key) already holds text shaped like a stand-in (like [NAME_1]). Restore could change it into a real value. Change that name in the raw file, then try again.")
				}
				if seen[k] {
					return nil, fail.New("json-dup", "This JSON file has the same name twice in one object, so I am not sure which value is right. Fix it, then try again.")
				}
				seen[k] = true
				v, err := readNode(dec, depth+1)
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, k)
				n.vals = append(n.vals, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, errJSON()
			}
			return n, nil
		case '[':
			n := &jnode{kind: 'a'}
			for dec.More() {
				v, err := readNode(dec, depth+1)
				if err != nil {
					return nil, err
				}
				n.vals = append(n.vals, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, errJSON()
			}
			return n, nil
		}
		return nil, errJSON()
	case string:
		return &jnode{kind: 's', raw: jsonString(t), str: t, isStr: true}, nil
	case json.Number:
		return &jnode{kind: 's', raw: t.String(), str: t.String()}, nil
	case bool:
		s := "false"
		if t {
			s = "true"
		}
		return &jnode{kind: 's', raw: s, str: s}, nil
	case nil:
		return &jnode{kind: 's', raw: "null", str: ""}, nil
	}
	return nil, errJSON()
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

// flatten walks a record and calls emit for each scalar with its path.
func flatten(prefix string, obj *jnode, emit func(path, v string)) error {
	for i, k := range obj.keys {
		// Dots and [] describe structure in the config; accepting them as
		// literal field names would let a keep rule select a different field.
		// Top-level record keys are values and do not pass through flatten.
		if strings.TrimSpace(k) == "" || strings.ContainsAny(k, ".[]") || k == KeyColumn {
			return fail.New("json-key", "A JSON field name is blank or uses reserved text (a dot, square brackets or (key)). That could give two fields the same config name. Rename that field, then try again.")
		}
		v := obj.vals[i]
		p := join(prefix, k)
		switch v.kind {
		case 's':
			emit(p, v.str)
		case 'o':
			if len(v.keys) == 0 {
				emit(p, "")
			}
			if err := flatten(p, v, emit); err != nil {
				return err
			}
		case 'a':
			if len(v.vals) == 0 {
				emit(p+"[]", "")
			}
			for _, e := range v.vals {
				switch e.kind {
				case 's':
					emit(p+"[]", e.str)
				case 'o':
					if len(e.keys) == 0 {
						emit(p+"[]", "")
					}
					if err := flatten(p+"[]", e, emit); err != nil {
						return err
					}
				default:
					return fail.New("json-shape", "This JSON file has a list inside a list. I cannot read that yet.")
				}
			}
		}
	}
	return nil
}

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// JSONString quotes s as a JSON string, without HTML escapes.
func JSONString(s string) string { return jsonString(s) }

func (d *Dataset) writeJSON(rw Rewriter) ([]byte, error) {
	var b strings.Builder
	dup := false
	indent := func(n int) {
		b.WriteString(strings.Repeat("  ", n))
	}
	var cell func(path string, n *jnode)
	cell = func(path string, n *jnode) {
		nv, changed := rw.Cell(d.colOf[path], n.str)
		if changed {
			b.WriteString(jsonString(nv))
		} else {
			b.WriteString(n.raw)
		}
	}
	var obj func(prefix string, n *jnode, depth int)
	var val func(path string, n *jnode, depth int)
	val = func(path string, n *jnode, depth int) {
		switch n.kind {
		case 's':
			cell(path, n)
		case 'o':
			obj(path, n, depth)
		case 'a':
			if len(n.vals) == 0 {
				b.WriteString("[]")
				return
			}
			b.WriteString("[\n")
			for i, e := range n.vals {
				indent(depth + 1)
				val(path+"[]", e, depth+1)
				if i < len(n.vals)-1 {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
			indent(depth)
			b.WriteString("]")
		}
	}
	obj = func(prefix string, n *jnode, depth int) {
		if len(n.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		seen := map[string]bool{}
		for i, k := range n.keys {
			indent(depth + 1)
			nk := rw.Name(k)
			if seen[nk] {
				dup = true
			}
			seen[nk] = true
			b.WriteString(jsonString(nk))
			b.WriteString(": ")
			val(join(prefix, k), n.vals[i], depth+1)
			if i < len(n.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		indent(depth)
		b.WriteString("}")
	}
	root := d.root
	if root.kind == 'a' {
		b.WriteString("[\n")
		for i, rec := range root.vals {
			indent(1)
			obj("", rec, 1)
			if i < len(root.vals)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("]\n")
	} else {
		b.WriteString("{\n")
		keyCol := d.colOf[KeyColumn]
		seen := map[string]bool{}
		for i, k := range root.keys {
			indent(1)
			nk, changed := rw.Cell(keyCol, k)
			if !changed {
				nk = k
			}
			if seen[nk] {
				dup = true
			}
			seen[nk] = true
			b.WriteString(jsonString(nk))
			b.WriteString(": ")
			obj("", root.vals[i], 1)
			if i < len(root.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("}\n")
	}
	if dup {
		return nil, fail.New("json-dup", "Two names in one JSON object would get the same stand-in, so the new file would be wrong. I wrote nothing. Rename one of them in the raw file.")
	}
	out := []byte(b.String())
	if !json.Valid(out) {
		return nil, fail.New("write", "I could not build valid JSON, so I wrote nothing.")
	}
	return out, nil
}
