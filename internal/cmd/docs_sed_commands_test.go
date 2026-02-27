package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
)

func TestParseFullExpr_SCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		repl    string
		global  bool
		nth     int
		wantErr bool
	}{
		{"basic", "s/foo/bar/", "foo", "bar", false, 0, false},
		{"global", "s/foo/bar/g", "foo", "bar", true, 0, false},
		{"case insensitive", "s/foo/bar/i", "(?i)foo", "bar", false, 0, false},
		{"multiline", "s/^foo/bar/m", "(?m)^foo", "bar", false, 0, false},
		{"all flags", "s/foo/bar/gim", "(?m)(?i)foo", "bar", true, 0, false},
		{"nth match 2", "s/foo/bar/2", "foo", "bar", false, 2, false},
		{"nth match 3 global", "s/foo/bar/3g", "foo", "bar", true, 3, false},
		{"empty", "", "", "", false, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseFullExpr(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.pattern, expr.pattern)
			assert.Equal(t, tt.global, expr.global)
			assert.Equal(t, tt.nth, expr.nthMatch)
			assert.Equal(t, byte(0), expr.command)
		})
	}
}

func TestParseDCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		wantErr bool
	}{
		{"basic", "d/foo/", "foo", false},
		{"case insensitive", "d/foo/i", "(?i)foo", false},
		{"multiline", "d/^line/m", "(?m)^line", false},
		{"regex", "d/^old.*$/", "^old.*$", false},
		{"empty pattern", "d//", "", true},
		{"too short", "d", "", true},
		{"not d command", "s/foo/bar/", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseDCommand(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.pattern, expr.pattern)
			assert.Equal(t, byte('d'), expr.command)
			assert.Empty(t, expr.replacement)
		})
	}
}

func TestParseAICommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		cmd     byte
		pattern string
		repl    string
		wantErr bool
	}{
		{"append basic", "a/match/new text/", 'a', "match", "new text", false},
		{"insert basic", "i/match/new text/", 'i', "match", "new text", false},
		{"append case insensitive", "a/match/text/i", 'a', "(?i)match", "text", false},
		{"empty text", "a/match//", 'a', "match", "", false},
		{"missing text", "a/match", 'a', "", "", true},
		{"too short", "a", 'a', "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseAICommand(tt.input, tt.cmd)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.pattern, expr.pattern)
			assert.Equal(t, tt.repl, expr.replacement)
			assert.Equal(t, tt.cmd, expr.command)
		})
	}
}

func TestParseYCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		source  string
		dest    string
		wantErr bool
	}{
		{"basic", "y/abc/xyz/", "abc", "xyz", false},
		{"vowels", "y/aeiou/AEIOU/", "aeiou", "AEIOU", false},
		{"unicode", "y/áéí/AEI/", "áéí", "AEI", false},
		{"length mismatch", "y/abc/xy/", "", "", true},
		{"empty source", "y//abc/", "", "", true},
		{"too short", "y", "", "", true},
		{"missing dest", "y/abc/", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseYCommand(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.source, expr.pattern)
			assert.Equal(t, tt.dest, expr.replacement)
			assert.Equal(t, byte('y'), expr.command)
		})
	}
}

func TestParseNthFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"no flag", "s/foo/bar/", 0},
		{"global only", "s/foo/bar/g", 0},
		{"nth 2", "s/foo/bar/2", 2},
		{"nth 3 global", "s/foo/bar/3g", 3},
		{"nth 10", "s/foo/bar/10", 10},
		{"nth 0 ignored", "s/foo/bar/0", 0},
		{"not sed", "d/foo/", 0},
		{"no flags section", "s/foo/bar", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseNthFlag(tt.input))
		})
	}
}

func TestParseFullExpr_CommandAmbiguity(t *testing.T) {
	// "data" starts with 'd' but should be parsed as s// not d//
	// This would fail if we don't check for alphanumeric delimiter
	tests := []struct {
		name    string
		input   string
		isCmd   bool
		cmd     byte
		wantErr bool
	}{
		{"d with slash delimiter", "d/pattern/", true, 'd', false},
		{"d command won't match s", "data_replace", false, 0, true}, // not valid sed
		{"a with slash", "a/pat/text/", true, 'a', false},
		{"i with slash", "i/pat/text/", true, 'i', false},
		{"y with slash", "y/abc/xyz/", true, 'y', false},
		{"s command", "s/foo/bar/", false, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseFullExpr(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.isCmd {
				assert.Equal(t, tt.cmd, expr.command)
			} else {
				assert.Equal(t, byte(0), expr.command)
			}
		})
	}
}

func TestSplitByDelim(t *testing.T) {
	tests := []struct {
		name  string
		input string
		delim byte
		want  []string
	}{
		{"basic", "a/b/c", '/', []string{"a", "b", "c"}},
		{"escaped", `a\/b/c`, '/', []string{"a/b", "c"}},
		{"empty parts", "//", '/', []string{"", "", ""}},
		{"no delim", "abc", '/', []string{"abc"}},
		{"custom delim", "a|b|c", '|', []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitByDelim(tt.input, tt.delim))
		})
	}
}

func TestApplyRegexFlags(t *testing.T) {
	assert.Equal(t, "foo", applyRegexFlags("foo", ""))
	assert.Equal(t, "(?i)foo", applyRegexFlags("foo", "i"))
	assert.Equal(t, "(?m)foo", applyRegexFlags("foo", "m"))
	assert.Equal(t, "(?m)(?i)foo", applyRegexFlags("foo", "im"))
	assert.Equal(t, "(?m)(?i)foo", applyRegexFlags("foo", "gim")) // g ignored here
}

func TestExtractParagraphText(t *testing.T) {
	// nil elements
	p := &docs.Paragraph{}
	assert.Equal(t, "", extractParagraphText(p))

	// single text run
	p = &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{TextRun: &docs.TextRun{Content: "Hello World\n"}},
		},
	}
	assert.Equal(t, "Hello World", extractParagraphText(p))

	// multiple text runs
	p = &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{TextRun: &docs.TextRun{Content: "Hello "}},
			{TextRun: &docs.TextRun{Content: "World\n"}},
		},
	}
	assert.Equal(t, "Hello World", extractParagraphText(p))
}

func TestExtractNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"g", 0},
		{"2", 2},
		{"g3", 3},
		{"2g", 2},
		{"10", 10},
		{"gi", 0},
		{"0", 0},
		{"-1", 1}, // extracts digits only, ignores minus
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, extractNumber(tt.input))
		})
	}
}

func TestEscapeUnescapeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"asterisk", "\\*bold\\*", "*bold*"},
		{"hash", "\\#heading", "#heading"},
		{"backslash", "\\\\path", "\\path"},
		{"newline", "line1\\nline2", "line1\nline2"},
		{"combined", "\\*\\#\\~\\`", "*#~`"},
		{"no escapes", "plain text", "plain text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeMarkdown(tt.input)
			restored := unescapeMarkdown(escaped)
			// After escape+unescape, \n should be real newline, others literal
			assert.Equal(t, tt.want, restored)
		})
	}
}

func TestIsAlphanumeric(t *testing.T) {
	assert.True(t, isAlphanumeric('a'))
	assert.True(t, isAlphanumeric('Z'))
	assert.True(t, isAlphanumeric('5'))
	assert.False(t, isAlphanumeric('/'))
	assert.False(t, isAlphanumeric('|'))
	assert.False(t, isAlphanumeric(' '))
}

func TestClassifyExpression(t *testing.T) {
	tests := []struct {
		name string
		expr sedExpr
		want string
	}{
		{"delete", sedExpr{command: 'd', pattern: "foo"}, "delete"},
		{"append", sedExpr{command: 'a', pattern: "foo"}, "append-after"},
		{"insert", sedExpr{command: 'i', pattern: "foo"}, "insert-before"},
		{"transliterate", sedExpr{command: 'y', pattern: "abc"}, "transliterate"},
		{"positional", sedExpr{pattern: "^"}, "positional"},
		{"positional end", sedExpr{pattern: "$"}, "positional"},
		{"manual", sedExpr{pattern: "foo", replacement: "**bar**"}, "manual"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyExpression(tt.expr))
		})
	}
}

func TestBuildTextStyleRequests(t *testing.T) {
	tests := []struct {
		name    string
		formats []string
		count   int
	}{
		{"empty", nil, 0},
		{"bold", []string{"bold"}, 1},
		{"bold+italic", []string{"bold", "italic"}, 1},
		{"code", []string{"code"}, 1},
		{"link", []string{"link:https://example.com"}, 1},
		{"heading only", []string{"heading1"}, 0}, // headings are paragraph-level
		{"bullet only", []string{"bullet"}, 0},    // bullets are paragraph-level
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildTextStyleRequests(tt.formats, 0, 10)
			assert.Equal(t, tt.count, len(reqs))
			if tt.count > 0 {
				assert.NotNil(t, reqs[0].UpdateTextStyle)
			}
		})
	}
}

func TestBuildParagraphStyleRequests(t *testing.T) {
	tests := []struct {
		name    string
		formats []string
		count   int
	}{
		{"empty", nil, 0},
		{"heading1", []string{"heading1"}, 1},
		{"heading6", []string{"heading6"}, 1},
		{"bullet", []string{"bullet"}, 1},
		{"numbered", []string{"numbered"}, 1},
		{"checkbox", []string{"checkbox"}, 1},
		{"heading+bullet", []string{"heading2", "bullet"}, 2},
		{"bold only", []string{"bold"}, 0}, // bold is text-level
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildParagraphStyleRequests(tt.formats, 0, 20)
			assert.Equal(t, tt.count, len(reqs))
		})
	}
}

func TestBuildCellReplaceRequests(t *testing.T) {
	// No delete, with text
	reqs := buildCellReplaceRequests(10, 10, "hello", nil)
	assert.Equal(t, 1, len(reqs)) // insert only
	assert.NotNil(t, reqs[0].InsertText)

	// With delete and text
	reqs = buildCellReplaceRequests(10, 15, "hello", nil)
	assert.Equal(t, 2, len(reqs)) // delete + insert
	assert.NotNil(t, reqs[0].DeleteContentRange)
	assert.NotNil(t, reqs[1].InsertText)

	// With delete, text, and format
	reqs = buildCellReplaceRequests(10, 15, "hello", []string{"bold"})
	assert.Equal(t, 3, len(reqs)) // delete + insert + format

	// Empty text
	reqs = buildCellReplaceRequests(10, 15, "", nil)
	assert.Equal(t, 1, len(reqs)) // delete only

	// No delete, no text
	reqs = buildCellReplaceRequests(10, 10, "", nil)
	assert.Equal(t, 0, len(reqs))
}

func TestBuildImageSizeSpec(t *testing.T) {
	assert.Nil(t, buildImageSizeSpec(&ImageSpec{URL: "http://x.com/img.png"}))

	size := buildImageSizeSpec(&ImageSpec{URL: "http://x.com/img.png", Width: 100})
	require.NotNil(t, size)
	assert.NotNil(t, size.Width)
	assert.Nil(t, size.Height)

	size = buildImageSizeSpec(&ImageSpec{URL: "http://x.com/img.png", Width: 100, Height: 200})
	require.NotNil(t, size)
	assert.NotNil(t, size.Width)
	assert.NotNil(t, size.Height)
}

func TestParseMarkdownReplacementFormats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		text    string
		formats []string
	}{
		{"plain", "hello", "hello", nil},
		{"bold", "**hello**", "hello", []string{"bold"}},
		{"italic", "*hello*", "hello", []string{"italic"}},
		{"bold+italic", "***hello***", "hello", []string{"bold", "italic"}},
		{"strike", "~~hello~~", "hello", []string{"strikethrough"}},
		{"code", "`hello`", "hello", []string{"code"}},
		{"heading1", "# Title", "Title", []string{"heading1"}},
		{"heading3", "### Sub", "Sub", []string{"heading3"}},
		{"bullet", "- Item", "Item", []string{"bullet"}},
		{"numbered", "1. Item", "Item", []string{"numbered"}},
		{"escaped asterisk", "\\*not bold\\*", "*not bold*", nil},
		{"escaped hash", "\\#not heading", "#not heading", nil},
		{"newline", "line1\\nline2", "line1\nline2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			assert.Equal(t, tt.text, text)
			assert.Equal(t, tt.formats, formats)
		})
	}
}

func TestParseMarkdownNewFormats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		text    string
		formats []string
	}{
		// Horizontal rule
		{"hrule dashes", "---", "\n", []string{"hrule"}},
		{"hrule asterisks", "***", "\n", []string{"hrule"}},
		{"hrule underscores", "___", "\n", []string{"hrule"}},
		{"not hrule 4 dashes", "----", "----", nil}, // 4+ not a rule
		{"not hrule text", "--text", "--text", nil},

		// Blockquote
		{"blockquote", "> This is a quote", "This is a quote", []string{"blockquote"}},
		{"not blockquote no space", ">nospace", ">nospace", nil},

		// Code block
		{"codeblock", "```\nfoo\nbar\n```", "foo\nbar\n", []string{"codeblock"}},
		{"codeblock with lang", "```go\nfoo\n```", "foo\n", []string{"codeblock"}},
		{"not codeblock backtick", "`inline`", "inline", []string{"code"}},

		// Nested lists
		{"nested bullet L1", "  - Item", "\tItem", []string{"bullet"}},
		{"nested bullet L2", "    - Item", "\t\tItem", []string{"bullet"}},
		{"nested numbered", "  1. Item", "\tItem", []string{"numbered"}},
		{"top level bullet", "- Item", "Item", []string{"bullet"}},

		// Footnote
		{"footnote", "[^This is footnote text]", "This is footnote text", []string{"footnote"}},
		{"not footnote link", "[text](url)", "text", []string{"link:url"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			assert.Equal(t, tt.text, text)
			assert.Equal(t, tt.formats, formats)
		})
	}
}

func TestContainsFormat(t *testing.T) {
	assert.True(t, containsFormat([]string{"bold", "italic"}, "bold"))
	assert.False(t, containsFormat([]string{"bold"}, "italic"))
	assert.False(t, containsFormat(nil, "bold"))
}

func TestParseHexColor(t *testing.T) {
	r, g, b, ok := parseHexColor("#FF0000")
	assert.True(t, ok)
	assert.InDelta(t, 1.0, r, 0.01)
	assert.InDelta(t, 0.0, g, 0.01)
	assert.InDelta(t, 0.0, b, 0.01)

	r, g, b, ok = parseHexColor("#00FF00")
	assert.True(t, ok)
	assert.InDelta(t, 0.0, r, 0.01)
	assert.InDelta(t, 1.0, g, 0.01)
	assert.InDelta(t, 0.0, b, 0.01)

	_, _, _, ok = parseHexColor("invalid")
	assert.False(t, ok)

	_, _, _, ok = parseHexColor("#GG0000")
	assert.False(t, ok)

	// Test #RGB shorthand expansion
	r, g, b, ok = parseHexColor("#F00")
	assert.True(t, ok)
	assert.InDelta(t, 1.0, r, 0.01)
	assert.InDelta(t, 0.0, g, 0.01)
	assert.InDelta(t, 0.0, b, 0.01)

	r, g, b, ok = parseHexColor("#0F0")
	assert.True(t, ok)
	assert.InDelta(t, 0.0, r, 0.01)
	assert.InDelta(t, 1.0, g, 0.01)
	assert.InDelta(t, 0.0, b, 0.01)
}
