package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/RKelln/standin/internal/config"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/store"
)

type reviewRow struct {
	match, column string
	col           config.Column
}

// rowsKeepsFirst lists every column rule: keeps, then scans, tokenizes and drops.
func rowsKeepsFirst(cfg *config.Config) []reviewRow {
	order := []string{config.Keep, config.Scan, config.Tokenize, config.Drop}
	var rows []reviewRow
	for _, a := range order {
		for _, f := range cfg.Files {
			for _, name := range f.ColumnNames() {
				if col := f.Columns[name]; col.Action == a {
					rows = append(rows, reviewRow{f.Match, name, col})
				}
			}
		}
	}
	return rows
}

func printReview(w io.Writer, cfg *config.Config) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ACTION\tFILES\tCOLUMN\tCATEGORY\tREASON")
	for _, r := range rowsKeepsFirst(cfg) {
		action := r.col.Action
		if action == config.Keep {
			action = "KEEP (real values)"
		}
		cat := r.col.Category
		if cat == "" {
			cat = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", action, r.match, r.column, cat, oneLine(r.col.Reason))
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "keep      the AI sees the real values. Only keep what is not private.")
	fmt.Fprintln(w, "scan      free text: known values and patterns are swapped; names I do not know stay in.")
	fmt.Fprintln(w, "tokenize  each value becomes a stand-in like [PERSON_1].")
	fmt.Fprintln(w, "drop      the value is blanked.")
	fmt.Fprintln(w, "Any column not listed is tokenized. A free-text column not listed is dropped.")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func runReview(c *ctx) error {
	cfg, err := c.loadConfig()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.env.Stdout, "Config: %s\n\n", config.FileName)
	printReview(c.env.Stdout, cfg)
	privDir, err := c.privateDir(false)
	if err != nil {
		return err
	}
	approved, why, err := c.approvalState(cfg, privDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.env.Stdout)
	switch {
	case !needsApproval(cfg):
		fmt.Fprintln(c.env.Stdout, "This config has no keep or scan, so it needs no approval.")
	case approved:
		fmt.Fprintln(c.env.Stdout, "Approved: yes. The keeps and scans above will be used.")
	default:
		fmt.Fprintf(c.env.Stdout, "Approved: no. %s `standin scrub` will not run until you run `standin approve` yourself, in your own terminal.\n", why)
		c.warnings++
	}
	return nil
}

func runApprove(c *ctx) error {
	if !c.env.StdinIsTerminal() {
		return fail.New("no-terminal", "approve must be run by a person typing in a terminal, not by an AI or a script. Open a terminal, go to the project folder, and run `standin approve`.")
	}
	cfg, err := c.loadConfig()
	if err != nil {
		return err
	}
	privDir, err := c.privateDir(false)
	if err != nil {
		return err
	}
	out := c.env.Stdout
	fmt.Fprintf(out, "Config: %s\n\n", config.FileName)
	printReview(out, cfg)
	fmt.Fprintln(out)
	in := bufio.NewReader(c.env.Stdin)
	ask := func(q string) (bool, error) {
		fmt.Fprint(out, q+" Type yes: ")
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return false, fail.New("no-answer", "I got no answer, so I approved nothing.")
		}
		a := strings.ToLower(strings.TrimSpace(line))
		return a == "yes" || a == "y", nil
	}
	var keeps, scans []string
	for _, r := range rowsKeepsFirst(cfg) {
		var q, verb string
		switch r.col.Action {
		case config.Keep:
			q, verb = fmt.Sprintf("Keep %s in %s? The AI will see these real values.", r.column, r.match), "keeping"
		case config.Scan:
			q, verb = fmt.Sprintf("Scan %s in %s? It is free text: names Standin does not know stay in, and the AI will see them.", r.column, r.match), "scanning"
		default:
			continue
		}
		ok, err := ask(q)
		if err != nil {
			return err
		}
		if !ok {
			return fail.New("not-approved", "You did not approve %s %s in %s, so I approved nothing. Change it in %s (for example to tokenize or drop), then run approve again.", verb, r.column, r.match, config.FileName)
		}
		if r.col.Action == config.Keep {
			keeps = append(keeps, r.match+":"+r.column)
		} else {
			scans = append(scans, r.match+":"+r.column)
		}
	}
	ok, err := ask("Use this config for scrub?")
	if err != nil {
		return err
	}
	if !ok {
		return fail.New("not-approved", "Not approved. Nothing changed.")
	}
	if err := store.Ensure(privDir); err != nil {
		return err
	}
	a := store.Approval{Version: 1, ConfigSHA256: cfg.Hash, ApprovedAt: time.Now().UTC().Format(time.RFC3339), Keeps: keeps, Scans: scans}
	if a.Keeps == nil {
		a.Keeps = []string{}
	}
	if err := store.WriteAll([]store.Out{{Path: filepath.Join(privDir, store.ApprovalFile), Data: store.MarshalApproval(a), Perm: 0o600}}); err != nil {
		return err
	}
	c.msg("Approved. If the config changes, you need to approve it again.")
	return nil
}

// StdinIsTerminal reports whether stdin is a terminal (a person typing).
func StdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// StdoutIsTerminal reports whether stdout is a terminal (a screen, not a pipe or file).
func StdoutIsTerminal() bool {
	st, err := os.Stdout.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
