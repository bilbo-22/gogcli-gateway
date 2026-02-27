package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

// fetchDoc creates a Docs service and fetches the document. Used by command implementations
// that need the full document structure (delete, append, insert).
func fetchDoc(ctx context.Context, account, id string) (*docs.Service, *docs.Document, error) {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return nil, nil, fmt.Errorf("create docs service: %w", err)
	}
	doc, err := getDoc(ctx, docsSvc, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get document: %w", err)
	}
	return docsSvc, doc, nil
}

// runDeleteCommand executes a d/pattern/ command, deleting all lines containing the pattern.
func (c *DocsSedCmd) runDeleteCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	re, err := expr.compilePattern()
	if err != nil {
		return fmt.Errorf("compile pattern: %w", err)
	}

	// Find paragraphs matching the pattern and collect their ranges for deletion
	var requests []*docs.Request
	deleted := 0

	// Walk in reverse so deletions don't shift indices
	if doc.Body == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (empty document)"})
	}
	elems := doc.Body.Content
	for i := len(elems) - 1; i >= 0; i-- {
		elem := elems[i]
		if elem.Paragraph == nil {
			continue
		}
		text := extractParagraphText(elem.Paragraph)
		if re.MatchString(text) {
			start := elem.StartIndex
			end := elem.EndIndex
			// Don't delete before the document body start
			if start < 1 {
				start = 1
			}
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: start,
						EndIndex:   end,
						SegmentId:  "",
					},
				},
			})
			deleted++
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (no matches)"})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (delete): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: fmt.Sprintf("%d lines", deleted)})
}

// runAppendCommand executes an a/pattern/text/ command, inserting text after each matching line.
func (c *DocsSedCmd) runAppendCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	return c.runInsertAroundMatch(ctx, u, account, id, expr, false)
}

// runInsertCommand executes an i/pattern/text/ command, inserting text before each matching line.
func (c *DocsSedCmd) runInsertCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	return c.runInsertAroundMatch(ctx, u, account, id, expr, true)
}

// runInsertAroundMatch implements both append-after and insert-before matching lines.
func (c *DocsSedCmd) runInsertAroundMatch(ctx context.Context, u *ui.UI, account, id string, expr sedExpr, before bool) error {
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	re, err := expr.compilePattern()
	if err != nil {
		return fmt.Errorf("compile pattern: %w", err)
	}

	// Process replacement text: convert \n to real newlines
	insertText := strings.ReplaceAll(expr.replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText += "\n"
	}

	// Collect insertion points (in reverse order to preserve indices)
	var insertPoints []int64
	if doc.Body == nil {
		cmd := "appended"
		if before {
			cmd = "inserted"
		}
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: cmd, Value: "0 (empty document)"})
	}
	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		text := extractParagraphText(elem.Paragraph)
		if re.MatchString(text) {
			if before {
				insertPoints = append(insertPoints, elem.StartIndex)
			} else {
				insertPoints = append(insertPoints, elem.EndIndex)
			}
		}
	}

	if len(insertPoints) == 0 {
		cmd := "appended"
		if before {
			cmd = "inserted"
		}
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: cmd, Value: "0 (no matches)"})
	}

	// Build requests in reverse document order
	var requests []*docs.Request
	for i := len(insertPoints) - 1; i >= 0; i-- {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertPoints[i]},
				Text:     insertText,
			},
		})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (insert): %w", err)
	}

	cmd := "appended"
	if before {
		cmd = "inserted"
	}
	return sedOutputOK(ctx, u, id, sedOutputKV{Key: cmd, Value: fmt.Sprintf("%d lines", len(insertPoints))})
}

// runTransliterate executes a y/source/dest/ command, replacing each character in source
// with the corresponding character in dest throughout the document.
func (c *DocsSedCmd) runTransliterate(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, _, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	sourceRunes := []rune(expr.pattern)
	destRunes := []rune(expr.replacement)

	// Use native FindReplace for each character pair
	var requests []*docs.Request
	for i, src := range sourceRunes {
		requests = append(requests, &docs.Request{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      string(src),
					MatchCase: true,
				},
				ReplaceText: string(destRunes[i]),
			},
		})
	}

	resp, err := batchUpdate(ctx, docsSvc, id, requests)
	if err != nil {
		return fmt.Errorf("batch update (transliterate): %w", err)
	}
	var replaced int
	if resp != nil {
		for _, reply := range resp.Replies {
			if reply.ReplaceAllText != nil {
				replaced += int(reply.ReplaceAllText.OccurrencesChanged)
			}
		}
	}

	return sedOutputOK(ctx, u, id,
		sedOutputKV{Key: "transliterated", Value: fmt.Sprintf("%d chars across %d pairs", replaced, len(sourceRunes))},
	)
}

// extractParagraphText returns the plain text content of a paragraph.
func extractParagraphText(p *docs.Paragraph) string {
	// Fast path: single text run (most common case) avoids Builder allocation.
	if len(p.Elements) == 1 && p.Elements[0].TextRun != nil {
		return strings.TrimRight(p.Elements[0].TextRun.Content, "\n")
	}
	var sb strings.Builder
	for _, elem := range p.Elements {
		if elem.TextRun != nil {
			sb.WriteString(elem.TextRun.Content)
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
