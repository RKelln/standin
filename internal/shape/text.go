package shape

import (
	"fmt"
	"sort"
	"strings"

	"github.com/RKelln/standin/internal/dataset"
)

// TextCounts are the facts about the free text of a file, as plain counts.
// The shape file shows them only as bands, never a word.
type TextCounts struct {
	Lines    int            // non-blank free-text lines
	Unusual  int            // candidate words in the unusual group
	NameLike int            // candidate words in the name-like group
	Calendar int            // day and month names
	Patterns map[string]int // pattern kind (email, phone number ...) to count
}

// TextMarkdown builds the shape file of a .md or .txt file: its fields, its
// tables, and its free text in count bands. shownPath is "" when the file
// name looks like data.
func TextMarkdown(d *dataset.Dataset, shownPath, version string, tc TextCounts) string {
	var b strings.Builder
	title := shownPath
	if title == "" {
		title = "a raw file (name hidden)"
	}
	fmt.Fprintf(&b, "# Shape of %s\n\n", title)
	b.WriteString("Made by Standin " + version + ". This file describes the data. It holds no real values or words from it.\n")
	b.WriteString("Counts and lengths are given in ranges, so a single value cannot be picked out.\n\n")
	if shownPath == "" {
		b.WriteString("**The file name looks like private data** (for example a name or a number). ")
		b.WriteString("Ask the person to rename the raw file to something plain, like `notes.md`, before going on.\n\n")
	}
	infos := Columns(d)
	var fields []ColumnInfo
	for i, c := range infos {
		if d.Source[i] == dataset.FromField {
			fields = append(fields, c)
		}
	}
	b.WriteString("- Kind: free text (Markdown or plain text)\n")
	fmt.Fprintf(&b, "- Text encoding: %s\n", d.Encoding)
	fmt.Fprintf(&b, "- Fields (`Label: value` lines): %d different labels\n", len(fields))
	fmt.Fprintf(&b, "- Markdown tables: %d\n\n", d.Tables)

	if len(fields) > 0 {
		b.WriteString("## Fields\n\n")
		b.WriteString("Each `Label: value` line is a field, named by its label. Every line with the same label is one field.\n\n")
		for _, c := range fields {
			fmt.Fprintf(&b, "### Field %s\n\n", mdCode(c.Label))
			writeColumn(&b, c)
		}
	}
	for t := 1; t <= d.Tables; t++ {
		var cols []ColumnInfo
		for i, c := range infos {
			if d.Source[i] == fmt.Sprintf("%s %d", dataset.FromTable, t) {
				cols = append(cols, c)
			}
		}
		var names []string
		for _, c := range cols {
			names = append(names, mdCode(tableLabel(c)))
		}
		fmt.Fprintf(&b, "## Table %d: %s\n\n", t, strings.Join(names, ", "))
		for _, c := range cols {
			fmt.Fprintf(&b, "### Table column %s\n\n", mdCode(tableLabel(c)))
			if c.HeaderFlag != "" {
				if c.HeaderFlag == "empty" {
					b.WriteString("- Header: blank. The config cannot name it, so Standin tokenizes it.\n")
				} else {
					b.WriteString("- **Header hidden:** it looks like private data. The config cannot name it, so Standin tokenizes it and swaps the header too.\n")
				}
				c.HeaderFlag = ""
			}
			writeColumn(&b, c)
		}
	}

	b.WriteString("## Free text\n\n")
	b.WriteString("Everything that is not a field or a table. These are counts only; Standin never shows the words to the AI.\n\n")
	fmt.Fprintf(&b, "- Lines of free text: %s\n", CountBand(tc.Lines))
	fmt.Fprintf(&b, "- Unusual words (not English words: names, brands, streets, short forms): %s\n", CountBand(tc.Unusual))
	fmt.Fprintf(&b, "- Name-like words (after Hi, Dear or Thanks, in a sign-off, first names): %s\n", CountBand(tc.NameLike))
	fmt.Fprintf(&b, "- Day and month names: %s\n", CountBand(tc.Calendar))
	var kinds []string
	for k := range tc.Patterns {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "- Looks like %s: %s\n", k, CountBand(tc.Patterns[k]))
	}
	b.WriteString("\n")
	b.WriteString(TextConfigHelp)
	return b.String()
}

func tableLabel(c ColumnInfo) string {
	if c.HeaderFlag != "" {
		return "(hidden header)"
	}
	return c.Label
}

// TextConfigHelp is the note at the end of a text file's shape file.
const TextConfigHelp = "## How to write the config for a text file\n\n" +
	"In `standin.config.json`, a rule for a `.md` or `.txt` file has up to three parts instead of `columns`:\n\n" +
	"- `fields`: one rule per field label above. Same actions as columns: `tokenize` (needs a `category`), `keep`, `drop`, `scan`. " +
	"A `scan` field also gets the word check (below). A field the config does not name is tokenized whole, or dropped if it reads like free text.\n" +
	"- `tables`: one rule per table column header above, used in every table of the file. Same actions and defaults as CSV columns.\n" +
	"- `text`: the rest of the text. `{\"action\": \"words\"}` is the word check: names, unusual words, known values and patterns " +
	"(emails, phone numbers ...) become stand-ins like `[NAME_1]`, except words the person chooses to keep in `standin approve`. " +
	"Other words stay, so a name that is also a common word may get through. `{\"action\": \"drop\"}` blanks the text. With no `text` rule, the text is dropped.\n" +
	"- `keep_groups` (only with `words`): `[\"calendar\"]` keeps day and month names. It is the only group. You cannot name words to keep: you never see them.\n\n" +
	"A person must approve every keep, every scan and every word check. For case notes or other text about people's lives, `drop` is the safer choice.\n" +
	"Categories: `PERSON ORG EMAIL PHONE ACCOUNT ID ADDRESS POSTAL KEY VALUE NAME`. Give each choice a short `reason`.\n\n" +
	"```json\n" +
	"{\n" +
	"  \"version\": 1,\n" +
	"  \"files\": [\n" +
	"    {\n" +
	"      \"match\": \"raw/inbox/*.md\",\n" +
	"      \"fields\": {\n" +
	"        \"From\":    {\"action\": \"tokenize\", \"category\": \"PERSON\", \"reason\": \"who wrote\"},\n" +
	"        \"Date\":    {\"action\": \"keep\", \"reason\": \"needed to sort\"},\n" +
	"        \"Subject\": {\"action\": \"scan\", \"reason\": \"short text; swap names\"}\n" +
	"      },\n" +
	"      \"tables\": {\n" +
	"        \"Price\":   {\"action\": \"keep\", \"reason\": \"prices are public\"}\n" +
	"      },\n" +
	"      \"text\": {\"action\": \"words\", \"keep_groups\": [\"calendar\"], \"reason\": \"emails; swap names\"}\n" +
	"    }\n" +
	"  ]\n" +
	"}\n" +
	"```\n\n" +
	"`match` is a pattern for file paths in the project, like `raw/*.md` or `raw/**/*.txt`. The first rule that matches a file is used.\n"
