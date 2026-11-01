// Command wordreport prints Standin's word-check candidates for each line of a
// JSON corpus, so a model can be compared against the deterministic check.
// Research tool: not part of the shipped CLI.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/words"
)

type item struct {
	ID   string   `json:"id"`
	Line string   `json:"line"`
	Gold []string `json:"gold"`
}

type candOut struct {
	Word    string `json:"word"`
	Key     string `json:"key"`
	Group   string `json:"group"`
	Reasons uint16 `json:"reasons"`
	Count   int    `json:"count"`
}

type patOut struct {
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

type lineOut struct {
	ID       string    `json:"id"`
	Line     string    `json:"line"`
	Cands    []candOut `json:"candidates"`
	Groups   []string  `json:"groups"`
	Patterns []patOut  `json:"patterns"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wordreport corpus.json")
		os.Exit(2)
	}
	if err := words.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "load word lists:", err)
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var items []item
	if err := json.Unmarshal(raw, &items); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	out := make([]lineOut, 0, len(items))
	for _, it := range items {
		f := words.NewFinder(nil)
		lines := []string{it.Line}
		if parts := strings.Split(it.Line, "\n"); len(parts) > 1 {
			lines = parts
		}
		f.AddLines(lines)
		lo := lineOut{ID: it.ID, Line: it.Line, Cands: []candOut{}, Patterns: []patOut{}}
		for _, h := range scan.FindPatterns(it.Line, nil) {
			lo.Patterns = append(lo.Patterns, patOut{Kind: h.Kind, Start: h.Start, End: h.End, Text: it.Line[h.Start:h.End]})
		}
		for _, c := range f.Candidates() {
			lo.Cands = append(lo.Cands, candOut{Word: c.Word, Key: c.Key, Group: c.Group(), Reasons: uint16(c.Reasons), Count: c.Count})
			lo.Groups = append(lo.Groups, c.Word+"="+c.Group())
		}
		out = append(out, lo)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
