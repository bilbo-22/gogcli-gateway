package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarSearchCmd struct {
	Query      string `arg:"" name:"query" help:"Search query"`
	From       string `name:"from" help:"Start time (RFC3339, date, or relative; default: 30 days ago)"`
	To         string `name:"to" help:"End time (RFC3339, date, or relative; default: 90 days from now)"`
	Today      bool   `name:"today" help:"Search today only (timezone-aware)"`
	Week       bool   `name:"week" help:"Search this week Mon-Sun (timezone-aware)"`
	CalendarID string `name:"calendar" help:"Calendar ID" default:"primary"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
}

func (c *CalendarSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(c.Query)
	if query == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	var from, to string

	// If convenience flags are used, use timezone-aware resolution
	if c.Today || c.Week || c.From != "" || c.To != "" {
		timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
			From:  c.From,
			To:    c.To,
			Today: c.Today,
			Week:  c.Week,
		})
		if err != nil {
			return err
		}
		from, to = timeRange.FormatRFC3339()
	} else {
		// Search-specific defaults: 30 days ago to 90 days from now
		loc, err := getUserTimezone(ctx, svc)
		if err != nil {
			return err
		}
		now := time.Now().In(loc)
		from = now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
		to = now.Add(90 * 24 * time.Hour).Format(time.RFC3339)
	}

	call := svc.Events.List(c.CalendarID).
		Q(query).
		TimeMin(from).
		TimeMax(to).
		MaxResults(c.Max).
		SingleEvents(true).
		OrderBy("startTime")

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"events": resp.Items,
			"query":  query,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range resp.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	_ = tw.Flush()
	return nil
}
