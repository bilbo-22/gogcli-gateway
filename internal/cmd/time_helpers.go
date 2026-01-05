package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

// TimeRangeFlags provides common time range options for calendar commands.
// Embed this struct in commands that need time range support.
type TimeRangeFlags struct {
	From     string `name:"from" help:"Start time (RFC3339, date, or relative: today, tomorrow, monday)"`
	To       string `name:"to" help:"End time (RFC3339, date, or relative)"`
	Today    bool   `name:"today" help:"Today only"`
	Tomorrow bool   `name:"tomorrow" help:"Tomorrow only"`
	Week     bool   `name:"week" help:"This week (Mon-Sun)"`
	Days     int    `name:"days" help:"Next N days" default:"0"`
}

// TimeRange represents a resolved time range with timezone.
type TimeRange struct {
	From     time.Time
	To       time.Time
	Location *time.Location
}

// getUserTimezone fetches the timezone from the user's primary calendar.
func getUserTimezone(ctx context.Context, svc *calendar.Service) (*time.Location, error) {
	cal, err := svc.CalendarList.Get("primary").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get primary calendar: %w", err)
	}

	if cal.TimeZone == "" {
		// Fall back to UTC if no timezone set
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(cal.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar timezone %q: %w", cal.TimeZone, err)
	}

	return loc, nil
}

// ResolveTimeRange resolves the time range flags into absolute times.
// If no flags are provided, defaults to "next 7 days" from now.
func ResolveTimeRange(ctx context.Context, svc *calendar.Service, flags TimeRangeFlags) (*TimeRange, error) {
	loc, err := getUserTimezone(ctx, svc)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(loc)
	var from, to time.Time

	// Handle convenience flags first
	switch {
	case flags.Today:
		from = startOfDay(now)
		to = endOfDay(now)
	case flags.Tomorrow:
		tomorrow := now.AddDate(0, 0, 1)
		from = startOfDay(tomorrow)
		to = endOfDay(tomorrow)
	case flags.Week:
		from = startOfWeek(now)
		to = endOfWeek(now)
	case flags.Days > 0:
		from = startOfDay(now)
		to = endOfDay(now.AddDate(0, 0, flags.Days-1))
	default:
		// Parse --from and --to, or use defaults
		if flags.From != "" {
			from, err = parseTimeExpr(flags.From, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --from: %w", err)
			}
		} else {
			from = now
		}

		if flags.To != "" {
			to, err = parseTimeExpr(flags.To, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --to: %w", err)
			}
		} else {
			// Default: 7 days from "from"
			to = from.AddDate(0, 0, 7)
		}
	}

	return &TimeRange{
		From:     from,
		To:       to,
		Location: loc,
	}, nil
}

// parseTimeExpr parses a time expression which can be:
// - RFC3339: 2026-01-05T14:00:00-08:00
// - Date only: 2026-01-05 (interpreted as start of day in user's timezone)
// - Relative: today, tomorrow, monday, next tuesday
func parseTimeExpr(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	expr = strings.TrimSpace(expr)

	// Try RFC3339 first (before lowercasing)
	if t, err := time.Parse(time.RFC3339, expr); err == nil {
		return t, nil
	}

	// Now lowercase for relative expressions
	exprLower := strings.ToLower(expr)

	// Try relative expressions
	switch exprLower {
	case "now":
		return now, nil
	case "today":
		return startOfDay(now), nil
	case "tomorrow":
		return startOfDay(now.AddDate(0, 0, 1)), nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1)), nil
	}

	// Try day of week (this week or next)
	if t, ok := parseWeekday(exprLower, now); ok {
		return t, nil
	}

	// Try date only (YYYY-MM-DD)
	if t, err := time.ParseInLocation("2006-01-02", expr, loc); err == nil {
		return t, nil
	}

	// Try date with time but no timezone
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", expr, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", expr, loc); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as time (try: 2026-01-05, today, tomorrow, monday)", expr)
}

// parseWeekday parses weekday expressions like "monday", "next tuesday"
func parseWeekday(expr string, now time.Time) (time.Time, bool) {
	expr = strings.TrimPrefix(expr, "next ")

	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"sun":       time.Sunday,
		"monday":    time.Monday,
		"mon":       time.Monday,
		"tuesday":   time.Tuesday,
		"tue":       time.Tuesday,
		"wednesday": time.Wednesday,
		"wed":       time.Wednesday,
		"thursday":  time.Thursday,
		"thu":       time.Thursday,
		"friday":    time.Friday,
		"fri":       time.Friday,
		"saturday":  time.Saturday,
		"sat":       time.Saturday,
	}

	targetDay, ok := weekdays[expr]
	if !ok {
		return time.Time{}, false
	}

	currentDay := now.Weekday()
	daysUntil := int(targetDay) - int(currentDay)
	if daysUntil <= 0 {
		daysUntil += 7 // Next week
	}

	return startOfDay(now.AddDate(0, 0, daysUntil)), true
}

// startOfDay returns the start of the day (00:00:00) in the given time's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// endOfDay returns the end of the day (23:59:59.999) in the given time's location.
func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// startOfWeek returns the start of the week (Monday 00:00:00) for the given time.
func startOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7 for ISO week
	}
	daysToMonday := weekday - 1
	monday := t.AddDate(0, 0, -daysToMonday)
	return startOfDay(monday)
}

// endOfWeek returns the end of the week (Sunday 23:59:59) for the given time.
func endOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysToSunday := 7 - weekday
	sunday := t.AddDate(0, 0, daysToSunday)
	return endOfDay(sunday)
}

// FormatRFC3339 formats a time as RFC3339 for API calls.
func (tr *TimeRange) FormatRFC3339() (from, to string) {
	return tr.From.Format(time.RFC3339), tr.To.Format(time.RFC3339)
}

// FormatHuman returns a human-readable description of the time range.
func (tr *TimeRange) FormatHuman() string {
	fromDate := tr.From.Format("Mon Jan 2")
	toDate := tr.To.Format("Mon Jan 2")

	if fromDate == toDate {
		return fromDate
	}
	return fmt.Sprintf("%s to %s", fromDate, toDate)
}
