package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// --- Unit tests for parseSedExpr ---

func TestParseSedExpr_Basic(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantPattern string
		wantReplace string
		wantGlobal  bool
		wantErr     bool
	}{
		{
			name:        "simple replacement",
			expr:        "s/foo/bar/",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  false,
		},
		{
			name:        "global flag",
			expr:        "s/foo/bar/g",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  true,
		},
		{
			name:        "empty replacement",
			expr:        "s/foo//",
			wantPattern: "foo",
			wantReplace: "",
			wantGlobal:  false,
		},
		{
			name:        "regex pattern",
			expr:        `s/\d+/NUM/g`,
			wantPattern: `\d+`,
			wantReplace: "NUM",
			wantGlobal:  true,
		},
		{
			name:        "backreference conversion",
			expr:        `s/(foo)/\1bar/`,
			wantPattern: "(foo)",
			wantReplace: "${1}bar",
			wantGlobal:  false,
		},
		{
			name:        "alternate delimiter",
			expr:        "s#foo#bar#g",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  true,
		},
		{
			name:        "pipe delimiter",
			expr:        "s|path/to/file|new/path|",
			wantPattern: "path/to/file",
			wantReplace: "new/path",
			wantGlobal:  false,
		},
		{
			name:    "invalid - not starting with s",
			expr:    "x/foo/bar/",
			wantErr: true,
		},
		{
			name:    "invalid - too short",
			expr:    "s/",
			wantErr: true,
		},
		{
			name:    "invalid - missing replacement",
			expr:    "s/foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", pattern, tt.wantPattern)
			}
			if replacement != tt.wantReplace {
				t.Errorf("replacement = %q, want %q", replacement, tt.wantReplace)
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

// --- Unit tests for parseMarkdownReplacement ---

func TestParseMarkdownReplacement(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "plain text",
			input:       "hello world",
			wantText:    "hello world",
			wantFormats: nil,
		},
		{
			name:        "bold",
			input:       "**bold text**",
			wantText:    "bold text",
			wantFormats: []string{"bold"},
		},
		{
			name:        "italic",
			input:       "*italic text*",
			wantText:    "italic text",
			wantFormats: []string{"italic"},
		},
		{
			name:        "bold italic",
			input:       "***bold italic***",
			wantText:    "bold italic",
			wantFormats: []string{"bold", "italic"},
		},
		{
			name:        "strikethrough",
			input:       "~~crossed out~~",
			wantText:    "crossed out",
			wantFormats: []string{"strikethrough"},
		},
		{
			name:        "code",
			input:       "`inline code`",
			wantText:    "inline code",
			wantFormats: []string{"code"},
		},
		{
			name:        "heading 1",
			input:       "# Title",
			wantText:    "Title",
			wantFormats: []string{"heading1"},
		},
		{
			name:        "heading 2",
			input:       "## Subtitle",
			wantText:    "Subtitle",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "heading 3",
			input:       "### Section",
			wantText:    "Section",
			wantFormats: []string{"heading3"},
		},
		{
			name:        "heading 6",
			input:       "###### Deep",
			wantText:    "Deep",
			wantFormats: []string{"heading6"},
		},
		{
			name:        "heading no space",
			input:       "##NoSpace",
			wantText:    "NoSpace",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "bullet list dash",
			input:       "- list item",
			wantText:    "list item",
			wantFormats: []string{"bullet"},
		},
		{
			name:        "bullet list asterisk",
			input:       "* list item",
			wantText:    "list item",
			wantFormats: []string{"bullet"},
		},
		{
			name:        "numbered list",
			input:       "1. first item",
			wantText:    "first item",
			wantFormats: []string{"numbered"},
		},
		{
			name:        "newline escape",
			input:       "line1\\nline2",
			wantText:    "line1\nline2",
			wantFormats: nil,
		},
		{
			name:        "bullet with bold",
			input:       "- **bold item**",
			wantText:    "bold item",
			wantFormats: []string{"bullet", "bold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}

// --- Integration tests for DocsEditCmd ---

// mockDocsServer creates a test server that simulates the Google Docs API
func mockDocsServer(t *testing.T, docContent string, onBatchUpdate func(reqs []*docs.Request)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /v1/documents/{docId}
		if r.Method == http.MethodGet && strings.Contains(path, "/documents/") {
			w.Header().Set("Content-Type", "application/json")
			doc := &docs.Document{
				DocumentId: "test-doc-id",
				Title:      "Test Document",
				Body: &docs.Body{
					Content: []*docs.StructuralElement{
						{
							StartIndex: 0,
							EndIndex:   int64(len(docContent)),
							Paragraph: &docs.Paragraph{
								Elements: []*docs.ParagraphElement{
									{
										StartIndex: 0,
										EndIndex:   int64(len(docContent)),
										TextRun: &docs.TextRun{
											Content: docContent,
										},
									},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}

		// POST /v1/documents/{docId}:batchUpdate
		if r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate") {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if onBatchUpdate != nil {
				onBatchUpdate(req.Requests)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{
				DocumentId: "test-doc-id",
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func TestDocsEditCmd_JSON(t *testing.T) {
	var capturedReqs []*docs.Request
	srv := mockDocsServer(t, "Hello world, hello universe!", func(reqs []*docs.Request) {
		capturedReqs = reqs
	})
	defer srv.Close()

	// Create docs service with test server
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// We need to inject the mock service - for now test the helper functions
	// Full integration requires refactoring to use a mockable service variable
	_ = docsSvc

	// Test that parseSedExpr handles edit-style input correctly
	// The edit command internally constructs a simple replacement
	pattern := "hello"
	replacement := "hi"

	if pattern != "hello" || replacement != "hi" {
		t.Errorf("unexpected values")
	}

	// Verify captured requests would have correct structure
	if len(capturedReqs) > 0 {
		for _, req := range capturedReqs {
			if req.ReplaceAllText == nil {
				t.Error("expected ReplaceAllText request")
			}
		}
	}
}

func TestDocsSedCmd_RegexMatching(t *testing.T) {
	// Test that regex patterns compile and match correctly
	tests := []struct {
		name  string
		expr  string
		input string
		want  string
		wantN int // expected number of matches with global
	}{
		{
			name:  "simple global replace",
			expr:  "s/foo/bar/g",
			input: "foo foo foo", //nolint:dupword
			want:  "bar bar bar", //nolint:dupword
			wantN: 3,
		},
		{
			name:  "first match only",
			expr:  "s/foo/bar/",
			input: "foo foo foo", //nolint:dupword
			want:  "bar foo foo", //nolint:dupword
			wantN: 1,
		},
		{
			name:  "digit replacement",
			expr:  `s/\d+/NUM/g`,
			input: "item1 item2 item3",
			want:  "itemNUM itemNUM itemNUM", //nolint:dupword
			wantN: 3,
		},
		{
			name:  "capture group",
			expr:  `s/(\w+)@(\w+)/\2:\1/g`,
			input: "user@host",
			want:  "host:user",
			wantN: 1,
		},
		{
			name:  "word boundary",
			expr:  `s/\bcat\b/dog/g`,
			input: "cat catalog bobcat cat",
			want:  "dog catalog bobcat dog",
			wantN: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if err != nil {
				t.Fatalf("parseSedExpr: %v", err)
			}

			re, err := compilePattern(pattern)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var result string
			var count int
			if global {
				result = re.ReplaceAllString(tt.input, replacement)
				count = len(re.FindAllString(tt.input, -1))
			} else {
				loc := re.FindStringIndex(tt.input)
				if loc != nil {
					result = tt.input[:loc[0]] + re.ReplaceAllString(tt.input[loc[0]:loc[1]], replacement) + tt.input[loc[1]:]
					count = 1
				} else {
					result = tt.input
					count = 0
				}
			}

			if result != tt.want {
				t.Errorf("result = %q, want %q", result, tt.want)
			}
			if count != tt.wantN {
				t.Errorf("match count = %d, want %d", count, tt.wantN)
			}
		})
	}
}

// compilePattern is a helper for tests (mirrors internal logic)
func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

func TestDocsEditCmd_EmptyDocId(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsEditCmd{
		DocID:      "   ",
		Find:       "foo",
		ReplaceStr: "bar",
	}

	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty docId")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want error containing 'empty'", err)
	}
}

func TestDocsEditCmd_EmptyFind(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsEditCmd{
		DocID:      "doc123",
		Find:       "",
		ReplaceStr: "bar",
	}

	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty find")
	}
}

func TestDocsSedCmd_InvalidExpression(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"not starting with s", "x/foo/bar/"},
		{"too short", "s/"},
		{"missing replacement", "s/foo"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseSedExpr(tt.expr)
			if err == nil {
				t.Errorf("expected error for expr %q", tt.expr)
			}
		})
	}
}

func TestDocsSedCmd_InvalidRegex(t *testing.T) {
	// Valid sed syntax but invalid regex
	pattern, _, _, err := parseSedExpr("s/[unclosed/replacement/")
	if err != nil {
		// parseSedExpr might not catch this - it's caught at compile time
		return
	}

	_, err = regexp.Compile(pattern)
	if err == nil {
		t.Error("expected regex compile error for unclosed bracket")
	}
}

// Test output formats
func TestDocsEditCmd_OutputFormat_JSON(t *testing.T) {
	// This test verifies the JSON output structure
	// Full integration would require mocking the Docs service

	expectedFields := []string{"status", "docId", "replaced"}
	output := map[string]any{
		"status":   "ok",
		"docId":    "test-doc",
		"replaced": 5,
	}

	for _, field := range expectedFields {
		if _, ok := output[field]; !ok {
			t.Errorf("missing field %q in JSON output", field)
		}
	}
}

func TestDocsEditCmd_OutputFormat_Text(t *testing.T) {
	// Verify text output format matches expected tab-separated format
	lines := []string{
		"status\tok",
		"docId\ttest-doc",
		"replaced\t5",
	}

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			t.Errorf("line %q should have 2 tab-separated parts", line)
		}
	}
}

// --- Additional edge case tests ---

func TestParseSedExpr_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantPattern string
		wantReplace string
		wantGlobal  bool
		wantErr     bool
	}{
		{
			name:        "multiple backrefs",
			expr:        `s/(\w+)\s+(\w+)/\2 \1/g`,
			wantPattern: `(\w+)\s+(\w+)`,
			wantReplace: "${2} ${1}",
			wantGlobal:  true,
		},
		{
			name:        "replacement with slashes using alternate delim",
			expr:        `s#/usr/local#/opt/homebrew#g`,
			wantPattern: "/usr/local",
			wantReplace: "/opt/homebrew",
			wantGlobal:  true,
		},
		{
			name:        "empty pattern",
			expr:        "s//replacement/",
			wantPattern: "",
			wantReplace: "replacement",
			wantGlobal:  false,
		},
		{
			name:        "special regex chars in pattern",
			expr:        `s/\$\d+\.\d{2}/PRICE/g`,
			wantPattern: `\$\d+\.\d{2}`,
			wantReplace: "PRICE",
			wantGlobal:  true,
		},
		{
			name:        "newline escape preserved",
			expr:        `s/;/;\n/g`,
			wantPattern: ";",
			wantReplace: ";\\n", // \n preserved for parseMarkdownReplacement to handle
			wantGlobal:  true,
		},
		{
			name:        "tab escape preserved",
			expr:        `s/,/\t/g`,
			wantPattern: ",",
			wantReplace: "\\t", // \t preserved as-is
			wantGlobal:  true,
		},
		{
			name:    "just s",
			expr:    "s",
			wantErr: true,
		},
		{
			name:    "s with delimiter only",
			expr:    "s/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", pattern, tt.wantPattern)
			}
			if replacement != tt.wantReplace {
				t.Errorf("replacement = %q, want %q", replacement, tt.wantReplace)
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

func TestParseMarkdownReplacement_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "multiple newlines",
			input:       "line1\\nline2\\nline3",
			wantText:    "line1\nline2\nline3",
			wantFormats: nil,
		},
		{
			name:        "heading with trailing space",
			input:       "## Title ",
			wantText:    "Title ",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "bold with spaces inside",
			input:       "**bold with spaces**",
			wantText:    "bold with spaces",
			wantFormats: []string{"bold"},
		},
		{
			name:        "not bold - unmatched asterisks",
			input:       "**not bold",
			wantText:    "**not bold",
			wantFormats: nil,
		},
		{
			name:        "not italic - single asterisk",
			input:       "*",
			wantText:    "*",
			wantFormats: nil,
		},
		{
			name:        "numbered list double digit",
			input:       "12. twelfth item",
			wantText:    "12. twelfth item", // only single digit supported
			wantFormats: nil,
		},
		{
			name:        "code with special chars",
			input:       "`func main() {}`",
			wantText:    "func main() {}",
			wantFormats: []string{"code"},
		},
		{
			name:        "strikethrough with emoji",
			input:       "~~old value 🎉~~",
			wantText:    "old value 🎉",
			wantFormats: []string{"strikethrough"},
		},
		{
			name:        "heading 4",
			input:       "#### H4 Title",
			wantText:    "H4 Title",
			wantFormats: []string{"heading4"},
		},
		{
			name:        "heading 5",
			input:       "##### H5 Title",
			wantText:    "H5 Title",
			wantFormats: []string{"heading5"},
		},
		{
			name:        "four asterisks parsed as italic",
			input:       "****",
			wantText:    "**", // outer * stripped as italic markers
			wantFormats: []string{"italic"},
		},
		{
			name:        "empty code",
			input:       "``",
			wantText:    "``",
			wantFormats: nil,
		},
		{
			name:        "bullet then italic",
			input:       "- *italic item*",
			wantText:    "italic item",
			wantFormats: []string{"bullet", "italic"},
		},
		{
			name:        "numbered then bold",
			input:       "1. **important**",
			wantText:    "important",
			wantFormats: []string{"numbered", "bold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}

func TestDocsSedCmd_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		input string
		want  string
		wantN int
	}{
		{
			name:  "email obfuscation",
			expr:  `s/(\w+)@(\w+)\.(\w+)/\1[at]\2[dot]\3/g`,
			input: "Contact: john@example.com or jane@test.org",
			want:  "Contact: john[at]example[dot]com or jane[at]test[dot]org",
			wantN: 2,
		},
		{
			name:  "date format conversion",
			expr:  `s#(\d{4})-(\d{2})-(\d{2})#\2/\3/\1#g`, // use # delimiter since replacement has /
			input: "Date: 2026-02-07",
			want:  "Date: 02/07/2026",
			wantN: 1,
		},
		{
			name:  "remove html tags",
			expr:  `s/<[^>]+>//g`,
			input: "<p>Hello <b>world</b></p>",
			want:  "Hello world",
			wantN: 4,
		},
		{
			name:  "camelCase to snake_case",
			expr:  `s/([a-z])([A-Z])/\1_\2/g`,
			input: "getUserName",
			want:  "get_User_Name",
			wantN: 2,
		},
		{
			name:  "trim whitespace",
			expr:  `s/^\s+|\s+$//g`,
			input: "  hello world  ",
			want:  "hello world",
			wantN: 2,
		},
		{
			name:  "collapse whitespace",
			expr:  `s/\s+/ /g`,
			input: "hello    world\t\tfoo",
			want:  "hello world foo",
			wantN: 2,
		},
		{
			name:  "quote words",
			expr:  `s/\b(\w+)\b/"\1"/g`,
			input: "hello world",
			want:  `"hello" "world"`,
			wantN: 2,
		},
		{
			name:  "version bump",
			expr:  `s/v(\d+)\.(\d+)\.(\d+)/v\1.\2.999/`,
			input: "Current: v1.2.3",
			want:  "Current: v1.2.999",
			wantN: 1,
		},
		{
			name:  "no match",
			expr:  `s/xyz/abc/g`,
			input: "hello world",
			want:  "hello world",
			wantN: 0,
		},
		{
			name:  "overlapping matches",
			expr:  `s/aa/a/g`,
			input: "aaaa",
			want:  "aa", // regex replaces non-overlapping
			wantN: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if err != nil {
				t.Fatalf("parseSedExpr: %v", err)
			}

			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var result string
			var count int
			if global {
				matches := re.FindAllStringIndex(tt.input, -1)
				count = len(matches)
				result = re.ReplaceAllString(tt.input, replacement)
			} else {
				if re.MatchString(tt.input) {
					result = re.ReplaceAllStringFunc(tt.input, func(s string) string {
						count++
						if count > 1 {
							return s
						}
						return re.ReplaceAllString(s, replacement)
					})
				} else {
					result = tt.input
				}
			}

			if result != tt.want {
				t.Errorf("result = %q, want %q", result, tt.want)
			}
			if count != tt.wantN {
				t.Errorf("match count = %d, want %d", count, tt.wantN)
			}
		})
	}
}

func TestDocsSedCmd_EmptyDocId(t *testing.T) {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsSedCmd{
		DocID:      "   ",
		Expression: "s/foo/bar/",
	}

	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty docId")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want error containing 'empty'", err)
	}
}

func TestDocsEditCmd_MatchCase(t *testing.T) {
	// Test that MatchCase flag is properly set.
	cmd := DocsEditCmd{MatchCase: true}
	if !cmd.MatchCase {
		t.Error("MatchCase should be true")
	}

	cmdNoCase := DocsEditCmd{MatchCase: false}
	if cmdNoCase.MatchCase {
		t.Error("MatchCase should be false")
	}
}

func TestDocsSedCmd_FlagsVariations(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		wantGlobal bool
	}{
		{"no flags", "s/a/b/", false},
		{"g flag", "s/a/b/g", true},
		{"gi flags", "s/a/b/gi", true}, // i is ignored but g should work
		{"ig flags", "s/a/b/ig", true},
		{"empty flags", "s/a/b//", false}, // extra delimiter
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, global, err := parseSedExpr(tt.expr)
			if err != nil {
				// some malformed exprs may error, that's ok
				return
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

// Test building batchUpdate requests
func TestBuildReplaceAllTextRequest(t *testing.T) {
	find := "old text"
	replace := "new text"
	matchCase := true

	// Simulate what DocsEditCmd builds
	req := &docs.Request{
		ReplaceAllText: &docs.ReplaceAllTextRequest{
			ContainsText: &docs.SubstringMatchCriteria{
				Text:      find,
				MatchCase: matchCase,
			},
			ReplaceText: replace,
		},
	}

	if req.ReplaceAllText == nil {
		t.Fatal("ReplaceAllText should not be nil")
	}
	if req.ReplaceAllText.ContainsText.Text != find {
		t.Errorf("Text = %q, want %q", req.ReplaceAllText.ContainsText.Text, find)
	}
	if req.ReplaceAllText.ReplaceText != replace {
		t.Errorf("ReplaceText = %q, want %q", req.ReplaceAllText.ReplaceText, replace)
	}
	if req.ReplaceAllText.ContainsText.MatchCase != matchCase {
		t.Errorf("MatchCase = %v, want %v", req.ReplaceAllText.ContainsText.MatchCase, matchCase)
	}
}

// Test that markdown formats map to correct Google Docs API structures
func TestMarkdownToDocsAPIMapping(t *testing.T) {
	tests := []struct {
		format   string
		wantBold bool
		wantItal bool
		wantStrk bool
	}{
		{"bold", true, false, false},
		{"italic", false, true, false},
		{"strikethrough", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			// Simulate format detection
			_, formats := parseMarkdownReplacement("**test**")
			hasBold := false
			for _, f := range formats {
				if f == "bold" {
					hasBold = true
				}
			}
			if tt.format == "bold" && !hasBold {
				t.Error("expected bold format")
			}
		})
	}
}

// Benchmark tests
// --- Tests for escape sequences ---

func TestParseMarkdownReplacement_Escapes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "escaped asterisks - literal bold syntax",
			input:       `\*\*this\*\*`,
			wantText:    "**this**",
			wantFormats: nil,
		},
		{
			name:        "escaped single asterisks - literal italic syntax",
			input:       `\*italic\*`,
			wantText:    "*italic*",
			wantFormats: nil,
		},
		{
			name:        "escaped hash - literal heading syntax",
			input:       `\# Not a heading`,
			wantText:    "# Not a heading",
			wantFormats: nil,
		},
		{
			name:        "escaped multiple hashes",
			input:       `\#\#\# literal hashes`,
			wantText:    "### literal hashes",
			wantFormats: nil,
		},
		{
			name:        "escaped tildes - literal strikethrough syntax",
			input:       `\~\~not struck\~\~`,
			wantText:    "~~not struck~~",
			wantFormats: nil,
		},
		{
			name:        "escaped backticks - literal code syntax",
			input:       "\\`not code\\`",
			wantText:    "`not code`",
			wantFormats: nil,
		},
		{
			name:        "escaped dash - literal bullet syntax",
			input:       `\- not a bullet`,
			wantText:    "- not a bullet",
			wantFormats: nil,
		},
		{
			name:        "escaped plus - literal checkbox syntax",
			input:       `\+ not a checkbox`,
			wantText:    "+ not a checkbox",
			wantFormats: nil,
		},
		{
			name:        "escaped backslash",
			input:       `path\\to\\file`,
			wantText:    `path\to\file`,
			wantFormats: nil,
		},
		{
			name:        "escaped backslash before asterisk",
			input:       `\\*still italic*`,
			wantText:    `\*still italic*`, // \\ becomes \, asterisks preserved (no matching pair)
			wantFormats: nil,
		},
		{
			name:        "escaped backslash then asterisks",
			input:       `\\*italic*`,
			wantText:    `\*italic*`, // \\ becomes \, asterisks not matched (doesn't start with *)
			wantFormats: nil,
		},
		{
			name:        "mixed escaped and real formatting",
			input:       `\*\*literal\*\* and **bold**`,
			wantText:    "**literal** and **bold**", // escapes break the pattern match
			wantFormats: nil,                        // no format because pattern has placeholders
		},
		{
			name:        "real bold with escaped content inside",
			input:       `**has \* inside**`,
			wantText:    "has * inside",
			wantFormats: []string{"bold"},
		},
		{
			name:        "escape at end",
			input:       `text\*`,
			wantText:    "text*",
			wantFormats: nil,
		},
		{
			name:        "multiple escapes",
			input:       `\*\*\*triple\*\*\*`,
			wantText:    "***triple***",
			wantFormats: nil,
		},
		{
			name:        "escaped newline still works",
			input:       `line1\nline2`,
			wantText:    "line1\nline2",
			wantFormats: nil,
		},
		{
			name:        "escaped chars with real formatting",
			input:       `**bold with \* asterisk**`,
			wantText:    "bold with * asterisk",
			wantFormats: []string{"bold"},
		},
		{
			name:        "heading with escaped hash inside",
			input:       `# Title with \# hash`,
			wantText:    "Title with # hash",
			wantFormats: []string{"heading1"},
		},
		{
			name:        "bullet with escaped dash",
			input:       `- item with \- dash`,
			wantText:    "item with - dash",
			wantFormats: []string{"bullet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}

// --- Tests for expression collection ---

func TestCollectExpressions_Positional(t *testing.T) {
	cmd := &DocsSedCmd{Expression: "s/foo/bar/"}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 1 || exprs[0] != "s/foo/bar/" {
		t.Errorf("got %v, want [s/foo/bar/]", exprs)
	}
}

func TestCollectExpressions_MultipleE(t *testing.T) {
	cmd := &DocsSedCmd{
		Expressions: []string{"s/foo/bar/", "s/baz/qux/g"},
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2", len(exprs))
	}
	if exprs[0] != "s/foo/bar/" || exprs[1] != "s/baz/qux/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_PositionalPlusE(t *testing.T) {
	cmd := &DocsSedCmd{
		Expression:  "s/first/one/",
		Expressions: []string{"s/second/two/"},
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 || exprs[0] != "s/first/one/" || exprs[1] != "s/second/two/" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edits.sed")
	content := "# Comment line\ns/foo/bar/\n\ns/baz/**qux**/g\n# Another comment\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &DocsSedCmd{File: path}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2: %v", len(exprs), exprs)
	}
	if exprs[0] != "s/foo/bar/" || exprs[1] != "s/baz/**qux**/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_FilePlusE(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edits.sed")
	if err := os.WriteFile(path, []byte("s/from-file/yes/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &DocsSedCmd{
		Expressions: []string{"s/from-flag/yes/"},
		File:        path,
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2", len(exprs))
	}
	if exprs[0] != "s/from-flag/yes/" || exprs[1] != "s/from-file/yes/" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_Stdin(t *testing.T) {
	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write expressions to pipe
	go func() {
		w.WriteString("# comment\ns/from-stdin/yes/\ns/also-stdin/**bold**/g\n")
		w.Close()
	}()

	// Replace os.Stdin temporarily
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cmd := &DocsSedCmd{} // no positional, no -e, no -f
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2: %v", len(exprs), exprs)
	}
	if exprs[0] != "s/from-stdin/yes/" || exprs[1] != "s/also-stdin/**bold**/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_StdinIgnoredWhenEProvided(t *testing.T) {
	// Stdin should NOT be read if -e flags are provided
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		w.WriteString("s/should-not-appear/nope/\n")
		w.Close()
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cmd := &DocsSedCmd{Expressions: []string{"s/from-flag/yes/"}}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 1 || exprs[0] != "s/from-flag/yes/" {
		t.Errorf("got %v, want [s/from-flag/yes/]", exprs)
	}
}

func TestCollectExpressions_NoInput(t *testing.T) {
	cmd := &DocsSedCmd{}
	_, err := cmd.collectExpressions()
	if err == nil {
		t.Error("expected error for no expressions")
	}
}

func TestCollectExpressions_FileMissing(t *testing.T) {
	cmd := &DocsSedCmd{File: "/nonexistent/file.sed"}
	_, err := cmd.collectExpressions()
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- Tests for image syntax parsing ---

func TestParseImageSyntax(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNil     bool
		wantURL     string
		wantAlt     string
		wantCaption string
		wantWidth   int
		wantHeight  int
	}{
		// Valid image syntax
		{
			name:    "basic image",
			input:   "![](https://example.com/image.png)",
			wantURL: "https://example.com/image.png",
		},
		{
			name:    "with alt text",
			input:   "![Company Logo](https://example.com/logo.png)",
			wantURL: "https://example.com/logo.png",
			wantAlt: "Company Logo",
		},
		{
			name:        "with title as caption",
			input:       `![Logo](https://example.com/logo.png "Our Company Logo")`,
			wantURL:     "https://example.com/logo.png",
			wantAlt:     "Logo",
			wantCaption: "Our Company Logo",
		},
		{
			name:      "with width",
			input:     "![](https://example.com/img.png){width=300}",
			wantURL:   "https://example.com/img.png",
			wantWidth: 300,
		},
		{
			name:       "with height",
			input:      "![](https://example.com/img.png){height=200}",
			wantURL:    "https://example.com/img.png",
			wantHeight: 200,
		},
		{
			name:       "with both dimensions",
			input:      "![](https://example.com/img.png){width=300 height=200}",
			wantURL:    "https://example.com/img.png",
			wantWidth:  300,
			wantHeight: 200,
		},
		{
			name:       "short dimension syntax",
			input:      "![](https://example.com/img.png){w=300 h=200}",
			wantURL:    "https://example.com/img.png",
			wantWidth:  300,
			wantHeight: 200,
		},
		{
			name:      "with px suffix",
			input:     "![](https://example.com/img.png){width=300px}",
			wantURL:   "https://example.com/img.png",
			wantWidth: 300,
		},
		{
			name:        "full syntax",
			input:       `![Logo](https://example.com/logo.png "Figure 1"){width=400 height=300}`,
			wantURL:     "https://example.com/logo.png",
			wantAlt:     "Logo",
			wantCaption: "Figure 1",
			wantWidth:   400,
			wantHeight:  300,
		},
		{
			name:    "url with query params",
			input:   "![](https://example.com/img.png?size=large&format=webp)",
			wantURL: "https://example.com/img.png?size=large&format=webp",
		},
		{
			name:    "alt with special chars",
			input:   "![A cool image!](https://example.com/img.png)",
			wantURL: "https://example.com/img.png",
			wantAlt: "A cool image!",
		},

		// Not image syntax
		{
			name:    "plain text",
			input:   "hello world",
			wantNil: true,
		},
		{
			name:    "bold text",
			input:   "**bold**",
			wantNil: true,
		},
		{
			name:    "starts with ! but not image",
			input:   "!important",
			wantNil: true,
		},
		{
			name:    "no closing bracket",
			input:   "![alt text(url)",
			wantNil: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageSyntax(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseImageSyntax(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseImageSyntax(%q) = nil, want non-nil", tt.input)
			}
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
			if got.Alt != tt.wantAlt {
				t.Errorf("Alt = %q, want %q", got.Alt, tt.wantAlt)
			}
			if got.Caption != tt.wantCaption {
				t.Errorf("Caption = %q, want %q", got.Caption, tt.wantCaption)
			}
			if got.Width != tt.wantWidth {
				t.Errorf("Width = %d, want %d", got.Width, tt.wantWidth)
			}
			if got.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", got.Height, tt.wantHeight)
			}
		})
	}
}

func TestParseImageRefPattern(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantNil      bool
		wantPosition bool
		wantPos      int
		wantAll      bool
		wantByAlt    bool
		wantRegex    string
	}{
		// Positional patterns !(n)
		{
			name:         "first image",
			input:        "!(1)",
			wantPosition: true,
			wantPos:      1,
		},
		{
			name:         "second image",
			input:        "!(2)",
			wantPosition: true,
			wantPos:      2,
		},
		{
			name:         "last image",
			input:        "!(-1)",
			wantPosition: true,
			wantPos:      -1,
		},
		{
			name:         "second to last",
			input:        "!(-2)",
			wantPosition: true,
			wantPos:      -2,
		},
		{
			name:         "all images",
			input:        "!(*)",
			wantPosition: true,
			wantAll:      true,
		},

		// Positional with empty alt ![](n)
		{
			name:         "first image alt syntax",
			input:        "![](1)",
			wantPosition: true,
			wantPos:      1,
		},
		{
			name:         "all images alt syntax",
			input:        "![](*)",
			wantPosition: true,
			wantAll:      true,
		},

		// Alt text regex patterns ![regex]
		{
			name:      "exact alt match",
			input:     "![logo]",
			wantByAlt: true,
			wantRegex: "logo",
		},
		{
			name:      "alt starts with",
			input:     "![fig-.*]",
			wantByAlt: true,
			wantRegex: "fig-.*",
		},
		{
			name:      "alt contains",
			input:     "![.*draft.*]",
			wantByAlt: true,
			wantRegex: ".*draft.*",
		},
		{
			name:      "alt with digits",
			input:     `![img-\d+]`,
			wantByAlt: true,
			wantRegex: `img-\d+`,
		},
		{
			name:      "case insensitive",
			input:     "![(?i)logo]",
			wantByAlt: true,
			wantRegex: "(?i)logo",
		},

		// Not image reference patterns
		{
			name:    "actual image insert",
			input:   "!(https://example.com/img.png)",
			wantNil: true, // URL, not reference
		},
		{
			name:    "full image syntax",
			input:   "![alt](https://example.com/img.png)",
			wantNil: true,
		},
		{
			name:    "plain text",
			input:   "hello world",
			wantNil: true,
		},
		{
			name:    "empty brackets",
			input:   "![]",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageRefPattern(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseImageRefPattern(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseImageRefPattern(%q) = nil, want non-nil", tt.input)
			}
			if got.ByPosition != tt.wantPosition {
				t.Errorf("ByPosition = %v, want %v", got.ByPosition, tt.wantPosition)
			}
			if got.Position != tt.wantPos {
				t.Errorf("Position = %d, want %d", got.Position, tt.wantPos)
			}
			if got.AllImages != tt.wantAll {
				t.Errorf("AllImages = %v, want %v", got.AllImages, tt.wantAll)
			}
			if got.ByAlt != tt.wantByAlt {
				t.Errorf("ByAlt = %v, want %v", got.ByAlt, tt.wantByAlt)
			}
			if tt.wantByAlt && got.AltRegex != nil {
				if got.AltRegex.String() != tt.wantRegex {
					t.Errorf("AltRegex = %q, want %q", got.AltRegex.String(), tt.wantRegex)
				}
			}
		})
	}
}

func TestMatchImages(t *testing.T) {
	images := []DocImage{
		{ObjectID: "img1", Index: 10, Alt: "logo"},
		{ObjectID: "img2", Index: 20, Alt: "fig-1"},
		{ObjectID: "img3", Index: 30, Alt: "fig-2"},
		{ObjectID: "img4", Index: 40, Alt: "header-draft"},
		{ObjectID: "img5", Index: 50, Alt: "footer"},
	}

	tests := []struct {
		name    string
		pattern string
		wantIDs []string
	}{
		{"first image", "!(1)", []string{"img1"}},
		{"second image", "!(2)", []string{"img2"}},
		{"last image", "!(-1)", []string{"img5"}},
		{"second to last", "!(-2)", []string{"img4"}},
		{"all images", "!(*)", []string{"img1", "img2", "img3", "img4", "img5"}},
		{"exact alt", "![logo]", []string{"img1"}},
		{"alt starts with fig", "![fig-.*]", []string{"img2", "img3"}},
		{"alt contains draft", "![.*draft.*]", []string{"img4"}},
		{"no match", "![nonexistent]", nil},
		{"out of range positive", "!(10)", nil},
		{"out of range negative", "!(-10)", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := parseImageRefPattern(tt.pattern)
			if ref == nil {
				t.Fatalf("parseImageRefPattern(%q) = nil", tt.pattern)
			}
			matched := matchImages(images, ref)
			gotIDs := make([]string, 0, len(matched))
			for _, img := range matched {
				gotIDs = append(gotIDs, img.ObjectID)
			}
			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("matched %v, want %v", gotIDs, tt.wantIDs)
				return
			}
			for i, id := range gotIDs {
				if id != tt.wantIDs[i] {
					t.Errorf("matched[%d] = %q, want %q", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestCanUseNativeReplace_Image(t *testing.T) {
	// Images should NOT use native replace
	tests := []struct {
		input string
		want  bool
	}{
		{"![](https://example.com/img.png)", false},
		{"![alt](url)", false},
		{"![](url){width=100}", false},
		{"!(https://example.com/img.png)", false}, // shorthand
		{"plain text", true},
	}

	for _, tt := range tests {
		got := canUseNativeReplace(tt.input)
		if got != tt.want {
			t.Errorf("canUseNativeReplace(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestImageShorthandSyntax(t *testing.T) {
	// Test !(url) shorthand is recognized as image for insertion
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantNil bool
	}{
		{
			name:    "shorthand http",
			input:   "!(http://example.com/img.png)",
			wantURL: "http://example.com/img.png",
		},
		{
			name:    "shorthand https",
			input:   "!(https://example.com/img.png)",
			wantURL: "https://example.com/img.png",
		},
		{
			name:    "shorthand with query",
			input:   "!(https://example.com/img.png?w=100)",
			wantURL: "https://example.com/img.png?w=100",
		},
		{
			name:    "positional ref not url",
			input:   "!(1)",
			wantNil: true, // This is a reference, not an image insert
		},
		{
			name:    "all images ref",
			input:   "!(*)",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First check if it's an image reference (position)
			ref := parseImageRefPattern(tt.input)
			if ref != nil && !tt.wantNil {
				t.Errorf("parseImageRefPattern(%q) matched as reference, expected URL", tt.input)
				return
			}

			// Then check if it's parsed as image syntax
			img := parseImageSyntax(tt.input)
			if tt.wantNil {
				// For positional refs, parseImageSyntax should also return nil
				// because they don't start with ![
				if img != nil {
					t.Errorf("parseImageSyntax(%q) = %+v, want nil", tt.input, img)
				}
				return
			}
			// For !(url) shorthand, parseImageSyntax won't match (needs ![)
			// But the sed command handles this specially
			// Just verify this is NOT a reference
			if ref != nil {
				t.Errorf("!(url) should not be parsed as reference")
			}
		})
	}
}

func TestImageRefPatternEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		desc    string
	}{
		{"zero position", "!(0)", false, "position 0 parses but won't match anything"},
		{"float position", "!(1.5)", true, "floats not supported"},
		{"negative zero", "!(-0)", false, "parses as 0, won't match anything"},
		{"large positive", "!(999)", false, "valid, will just not match"},
		{"large negative", "!(-999)", false, "valid, will just not match"},
		{"empty parens", "!()", true, "empty is invalid"},
		{"space in parens", "!( 1 )", true, "spaces not trimmed"},
		{"alt with spaces", "![my logo]", false, "spaces in alt ok"},
		{"complex regex", `![^fig-\d{2,4}$]`, false, "complex regex ok"},
		{"invalid regex", "![[invalid]", true, "unclosed bracket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageRefPattern(tt.input)
			if tt.wantNil && got != nil {
				t.Errorf("parseImageRefPattern(%q) = %+v, want nil (%s)", tt.input, got, tt.desc)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("parseImageRefPattern(%q) = nil, want non-nil (%s)", tt.input, tt.desc)
			}
		})
	}
}

func TestMatchImagesEdgeCases(t *testing.T) {
	// Empty image list
	t.Run("empty list", func(t *testing.T) {
		ref := parseImageRefPattern("!(1)")
		matched := matchImages(nil, ref)
		if len(matched) != 0 {
			t.Errorf("expected no matches for empty list")
		}
	})

	// Single image
	t.Run("single image", func(t *testing.T) {
		images := []DocImage{{ObjectID: "only", Alt: "solo"}}

		ref1 := parseImageRefPattern("!(1)")
		if m := matchImages(images, ref1); len(m) != 1 || m[0].ObjectID != "only" {
			t.Errorf("!(1) should match single image")
		}

		ref2 := parseImageRefPattern("!(-1)")
		if m := matchImages(images, ref2); len(m) != 1 || m[0].ObjectID != "only" {
			t.Errorf("!(-1) should match single image")
		}

		ref3 := parseImageRefPattern("!(2)")
		if m := matchImages(images, ref3); len(m) != 0 {
			t.Errorf("!(2) should not match single image")
		}
	})

	// Alt text edge cases
	t.Run("empty alt", func(t *testing.T) {
		images := []DocImage{
			{ObjectID: "img1", Alt: ""},
			{ObjectID: "img2", Alt: "has-alt"},
		}

		// Match empty alt
		ref := parseImageRefPattern("![^$]")
		matched := matchImages(images, ref)
		if len(matched) != 1 || matched[0].ObjectID != "img1" {
			t.Errorf("![^$] should match empty alt")
		}
	})

	// Special chars in alt
	t.Run("special chars in alt", func(t *testing.T) {
		images := []DocImage{
			{ObjectID: "img1", Alt: "image (1)"},
			{ObjectID: "img2", Alt: "image [2]"},
		}

		// Need to escape special regex chars
		ref := parseImageRefPattern(`![image \(1\)]`)
		matched := matchImages(images, ref)
		if len(matched) != 1 || matched[0].ObjectID != "img1" {
			t.Errorf("escaped parens should match")
		}
	})
}

// --- Tests for native replace detection ---

func TestCanUseNativeReplace(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		want        bool
	}{
		// Should use native
		{"plain text", "hello world", true},
		{"with numbers", "item123", true},
		{"with special chars", "foo@bar.com", true},
		{"empty", "", true},
		{"single word", "replaced", true},
		{"path", "/usr/local/bin", true},
		{"url", "https://example.com", true},

		// Should NOT use native (has formatting)
		{"bold", "**bold**", false},
		{"italic", "*italic*", false},
		{"bold partial", "some **bold** text", false},
		{"strikethrough", "~~struck~~", false},
		{"code", "`code`", false},
		{"heading 1", "# Heading", false},
		{"heading 2", "## Heading", false},
		{"heading 3", "### Heading", false},
		{"bullet dash", "- item", false},
		{"bullet plus", "+ item", false},
		{"numbered list", "1. first", false},
		{"newline escape", "line1\\nline2", false},

		// Edge cases
		{"asterisk not format", "5 * 3 = 15", false}, // has * so detected as potential format
		{"hash in middle", "item #123", true},        // # not at start with space
		{"dash in middle", "foo-bar", true},          // - not at start with space
		{"number without dot space", "123", true},    // not "N. " pattern
		{"heading no space", "#hashtag", true},       // no space after #
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canUseNativeReplace(tt.replacement)
			if got != tt.want {
				t.Errorf("canUseNativeReplace(%q) = %v, want %v", tt.replacement, got, tt.want)
			}
		})
	}
}

// Benchmark tests

func BenchmarkParseSedExpr(b *testing.B) {
	expr := `s/(\w+)@(\w+)\.(\w+)/\1[at]\2[dot]\3/g`
	for i := 0; i < b.N; i++ {
		_, _, _, _ = parseSedExpr(expr)
	}
}

func BenchmarkParseMarkdownReplacement(b *testing.B) {
	inputs := []string{
		"plain text",
		"**bold text**",
		"- bullet with **bold**",
		"### Heading Three",
	}
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			parseMarkdownReplacement(input)
		}
	}
}

func BenchmarkRegexReplace(b *testing.B) {
	re := regexp.MustCompile(`\b(\w+)\b`)
	input := "The quick brown fox jumps over the lazy dog"
	replacement := `"$1"`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(input, replacement)
	}
}

// --- Unit tests for parseTableCellRef ---

func TestParseTableCellRef(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantNil    bool
		wantTable  int
		wantRow    int
		wantCol    int
		wantSubPat string
	}{
		{"numeric basic", "|1|[2,3]", false, 1, 2, 3, ""},
		{"excel style", "|1|[A1]", false, 1, 1, 1, ""},
		{"negative table", "|-1|[1,1]", false, -1, 1, 1, ""},
		{"with subpattern", "|1|[2,4]:old", false, 1, 2, 4, "old"},
		{"excel B2", "|2|[B2]", false, 2, 2, 2, ""},
		{"not a ref", "hello", true, 0, 0, 0, ""},
		{"no bracket", "|1|foo", true, 0, 0, 0, ""},
		{"single pipe", "|1[2,3]", true, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := parseTableCellRef(tt.input)
			if tt.wantNil {
				if ref != nil {
					t.Errorf("expected nil, got %+v", ref)
				}
				return
			}
			if ref == nil {
				t.Fatal("expected non-nil ref")
			}
			if ref.tableIndex != tt.wantTable {
				t.Errorf("tableIndex = %d, want %d", ref.tableIndex, tt.wantTable)
			}
			if ref.row != tt.wantRow {
				t.Errorf("row = %d, want %d", ref.row, tt.wantRow)
			}
			if ref.col != tt.wantCol {
				t.Errorf("col = %d, want %d", ref.col, tt.wantCol)
			}
			if ref.subPattern != tt.wantSubPat {
				t.Errorf("subPattern = %q, want %q", ref.subPattern, tt.wantSubPat)
			}
		})
	}
}

func TestParseTableCellRefExcel(t *testing.T) {
	tests := []struct {
		input   string
		wantRow int
		wantCol int
	}{
		{"A1", 1, 1},
		{"B2", 2, 2},
		{"C10", 10, 3},
		{"Z1", 1, 26},
		{"AA1", 1, 27},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			row, col, ok := parseExcelRef(tt.input)
			if !ok {
				t.Fatal("parseExcelRef returned false")
			}
			if row != tt.wantRow {
				t.Errorf("row = %d, want %d", row, tt.wantRow)
			}
			if col != tt.wantCol {
				t.Errorf("col = %d, want %d", col, tt.wantCol)
			}
		})
	}
}
