package cmd

import (
	"context"
	"errors"
	"io"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/sheetsconditional"
	"github.com/steipete/gogcli/internal/sheetsformat"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsConditionalCmd struct {
	List  SheetsConditionalListCmd  `cmd:"" default:"withargs" help:"List conditional formatting rules"`
	Add   SheetsConditionalAddCmd   `cmd:"" name:"add" aliases:"create,new" help:"Add a conditional formatting rule"`
	Clear SheetsConditionalClearCmd `cmd:"" name:"clear" aliases:"delete,rm,remove" help:"Remove conditional formatting rules"`
}

type SheetsConditionalAddCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"A1 range with sheet name (e.g. Sheet1!A2:J)"`
	Type          string `name:"type" required:"" help:"Rule type: text-eq|text-contains|text-starts-with|text-ends-with|number-eq|number-gt|number-gte|number-lt|number-lte|blank|not-blank|custom-formula"`
	Expr          string `name:"expr" help:"Expression value or custom formula (omit for blank/not-blank)"`
	FormatJSON    string `name:"format-json" required:"" help:"CellFormat JSON (inline or @file)"`
	FormatFields  string `name:"format-fields" help:"Format field mask for force-sending zero/false fields (e.g. backgroundColor,textFormat.bold)"`
	Index         int64  `name:"index" help:"Insert rule at this priority index" default:"0"`
}

func (c *SheetsConditionalAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	if c.Index < 0 {
		return usage("--index must be zero or greater")
	}

	parsedRange, err := parseSheetRange(rangeSpec, "conditional-format")
	if err != nil {
		return err
	}
	format, formatFields, err := parseConditionalFormat(c.FormatJSON, c.FormatFields, stdinReader(ctx))
	if err != nil {
		return err
	}
	conditionType, values, err := sheetsconditional.BuildCondition(strings.TrimSpace(c.Type), strings.TrimSpace(c.Expr))
	if err != nil {
		return sheetsConditionalPlannerError(err)
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.conditional-format.add", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           conditionType,
		"values":         values,
		"format_fields":  formatFields,
		"index":          c.Index,
	}); dryErr != nil {
		return dryErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(parsedRange, sheetIDs, "conditional-format")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			sheetsconditional.BuildAddRequest(gridRange, conditionType, values, format, c.Index),
		},
	}

	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"type":          conditionType,
			"index":         c.Index,
		})
	}
	u.Out().Linef("Added conditional format rule to %s", rangeSpec)
	return nil
}

type SheetsConditionalListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Only list rules from this sheet"`
}

func (c *SheetsConditionalListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),conditionalFormats)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	items := sheetsconditional.RuleItems(resp, strings.TrimSpace(c.Sheet))
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"rules": items})
	}
	if len(items) == 0 {
		u.Err().Println("No conditional format rules")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, sheetsConditionalColumns())
}

type SheetsConditionalClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" required:"" help:"Sheet name"`
	Index         string `name:"index" help:"Rule index to remove"`
	All           bool   `name:"all" help:"Remove all conditional formatting rules from the sheet"`
}

func (c *SheetsConditionalClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetName := strings.TrimSpace(c.Sheet)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if sheetName == "" {
		return usage("empty --sheet")
	}
	if !c.All && strings.TrimSpace(c.Index) == "" {
		return usage("provide --index or --all")
	}
	if c.All && strings.TrimSpace(c.Index) != "" {
		return usage("use either --index or --all, not both")
	}
	if err := sheetsconditional.ValidateClearIndex(strings.TrimSpace(c.Index)); err != nil {
		return sheetsConditionalPlannerError(err)
	}

	if flags != nil && flags.DryRun {
		return dryRunAndConfirmDestructive(ctx, flags, "sheets.conditional-format.clear", map[string]any{
			"spreadsheet_id": spreadsheetID,
			"sheet":          sheetName,
			"index":          strings.TrimSpace(c.Index),
			"all":            c.All,
		}, "remove conditional format rules from "+sheetName)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),conditionalFormats)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	sheetID, count, err := sheetsconditional.SheetRuleCount(resp, sheetName)
	if err != nil {
		return sheetsConditionalPlannerError(err)
	}

	requests, err := sheetsconditional.BuildDeleteRequests(sheetID, count, strings.TrimSpace(c.Index), c.All)
	if err != nil {
		return sheetsConditionalPlannerError(err)
	}
	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No conditional format rules to remove")
		return nil
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "sheets.conditional-format.clear", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          sheetName,
		"index":          strings.TrimSpace(c.Index),
		"all":            c.All,
		"removed":        len(requests),
	}, "remove conditional format rules from "+sheetName); err != nil {
		return err
	}

	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheet":         sheetName,
			"removed":       len(requests),
		})
	}
	ui.FromContext(ctx).Out().Linef("Removed %d conditional format rules from %s", len(requests), sheetName)
	return nil
}

func parseConditionalFormat(formatJSON, formatMask string, input io.Reader) (*sheets.CellFormat, string, error) {
	b, err := resolveInlineOrFileBytes(formatJSON, input)
	if err != nil {
		return nil, "", usagef("read --format-json: %v", err)
	}
	var format sheets.CellFormat
	if err := sheetsformat.Decode(b, &format); err != nil {
		return nil, "", usagef("invalid --format-json: %v", err)
	}
	formatFields := strings.TrimSpace(formatMask)
	if formatFields != "" {
		if sheetsformat.HasBordersTypo(formatFields) {
			return nil, "", usage(`invalid --format-fields: found "boarders"; use "borders"`)
		}
		normalized, formatPaths := sheetsformat.NormalizeMask(formatFields)
		formatFields = strings.TrimPrefix(normalized, sheetsformat.UserEnteredFormatPrefix+".")
		formatFields = strings.ReplaceAll(formatFields, ","+sheetsformat.UserEnteredFormatPrefix+".", ",")
		if err := sheetsformat.ApplyForceSendFields(&format, formatPaths); err != nil {
			return nil, "", usage(err.Error())
		}
	}
	return &format, formatFields, nil
}

func sheetsConditionalPlannerError(err error) error {
	var validationErr sheetsconditional.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}

	return err
}
