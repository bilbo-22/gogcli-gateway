package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

type CalendarUnsubscribeCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID or alias to remove from your calendar list"`
}

func (c *CalendarUnsubscribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	preparedID, err := prepareCalendarID(store, calendarID, false)
	if err != nil {
		return err
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "calendar.unsubscribe", map[string]any{
		"calendar_id": preparedID,
	}, fmt.Sprintf("unsubscribe from calendar %s", preparedID)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	resolvedID, err := resolveCalendarID(ctx, svc, preparedID)
	if err != nil {
		return err
	}

	if err := svc.CalendarList.Delete(resolvedID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("unsubscribe from calendar %s: %w", resolvedID, err)
	}

	return writeResult(ctx, u,
		kv("unsubscribed", true),
		kv("calendarId", resolvedID),
	)
}

type CalendarDeleteCalendarCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Owned secondary calendar ID or alias"`
}

func (c *CalendarDeleteCalendarCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	preparedID, err := prepareCalendarID(store, calendarID, false)
	if err != nil {
		return err
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "calendar.delete-calendar", map[string]any{
		"calendar_id": preparedID,
	}, fmt.Sprintf("permanently delete secondary calendar %s", preparedID)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	resolvedID, err := resolveCalendarID(ctx, svc, preparedID)
	if err != nil {
		return err
	}

	if err := svc.Calendars.Delete(resolvedID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete secondary calendar %s: %w", resolvedID, err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("calendarId", resolvedID),
	)
}
