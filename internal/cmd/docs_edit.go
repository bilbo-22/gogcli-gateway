package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsEditCmd does find/replace in a Google Doc
type DocsEditCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Find       string `name:"find" short:"f" help:"Text to find" required:""`
	ReplaceStr string `name:"replace" short:"r" help:"Text to replace with" required:""`
	MatchCase  bool   `name:"match-case" help:"Case-sensitive matching" default:"true"`
}

func (c *DocsEditCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	if c.Find == "" {
		return usage("empty find text")
	}

	// Create Docs service
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	// Build replace request
	requests := []*docs.Request{
		{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      c.Find,
					MatchCase: c.MatchCase,
				},
				ReplaceText: c.ReplaceStr,
			},
		},
	}

	// Execute batch update
	resp, err := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	// Get count of replacements
	replaced := int64(0)
	if resp != nil && len(resp.Replies) > 0 && resp.Replies[0].ReplaceAllText != nil {
		replaced = resp.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"status":   "ok",
			"docId":    id,
			"replaced": replaced,
		})
	}

	u.Out().Printf("status\tok")
	u.Out().Printf("docId\t%s", id)
	u.Out().Printf("replaced\t%d", replaced)
	return nil
}
