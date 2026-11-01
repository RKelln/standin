// Probe 2: hugot + pure-Go GoMLX backend on bert-small-pii, over a corpus.
// Emits the same JSON schema as the Python runners so score2.py can score it.
//
//	go run . -corpus ../../pii-eval/corpus_hard.json -model ./model -out hugot_hard.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
)

type item struct {
	ID   string   `json:"id"`
	Line string   `json:"line"`
	Gold []string `json:"gold"`
}

type span struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

type row struct {
	ID    string   `json:"id"`
	Line  string   `json:"line"`
	Gold  []string `json:"gold"`
	Spans []span   `json:"spans"`
}

func main() {
	corpusPath := flag.String("corpus", "", "corpus json")
	modelPath := flag.String("model", "", "model dir")
	outPath := flag.String("out", "", "output json")
	flag.Parse()

	raw, err := os.ReadFile(*corpusPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var items []item
	if err := json.Unmarshal(raw, &items); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ctx := context.Background()
	session, err := hugot.NewGoSession(ctx)
	if err != nil {
		fmt.Println("session:", err)
		os.Exit(1)
	}
	defer func() { _ = session.Destroy() }()

	pipe, err := hugot.NewPipeline(session, backends.PipelineConfig[*pipelines.TokenClassificationPipeline]{
		ModelPath:    *modelPath,
		Name:         "pii",
		OnnxFilename: "model.onnx",
	})
	if err != nil {
		fmt.Println("pipeline:", err)
		os.Exit(1)
	}

	inputs := make([]string, len(items))
	for i, it := range items {
		inputs[i] = it.Line
	}
	// warm up: the first calls compile graphs for the shapes they see
	_, _ = pipe.RunPipeline(ctx, inputs[:min(2, len(inputs))])

	start := time.Now()
	out, err := pipe.RunPipeline(ctx, inputs)
	if err != nil {
		fmt.Println("run:", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	rows := make([]row, 0, len(items))
	for i, it := range items {
		sp := []span{}
		ents := append([]pipelines.Entity(nil), out.Entities[i]...)
		sort.Slice(ents, func(a, b int) bool { return ents[a].Start < ents[b].Start })
		for _, e := range ents {
			s, en := int(e.Start), int(e.End)
			if s < 0 || en > len(it.Line) || s >= en {
				continue
			}
			sp = append(sp, span{Label: e.Entity, Start: s, End: en, Text: it.Line[s:en]})
		}
		rows = append(rows, row{ID: it.ID, Line: it.Line, Gold: it.Gold, Spans: sp})
	}
	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	if err := enc.Encode(rows); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	_ = f.Close()
	fmt.Printf("%d lines in %v (%.1f ms/line)\n", len(items), elapsed,
		float64(elapsed.Microseconds())/1000/float64(len(items)))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
