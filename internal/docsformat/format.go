package docsformat

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
)

const codeBackgroundGrey = 0.95

const (
	namedStyleNormalText = "NORMAL_TEXT"
	namedStyleTitle      = "TITLE"
	namedStyleSubtitle   = "SUBTITLE"
	namedStyleHeading1   = "HEADING_1"
	namedStyleHeading2   = "HEADING_2"
	namedStyleHeading3   = "HEADING_3"
	namedStyleHeading4   = "HEADING_4"
	namedStyleHeading5   = "HEADING_5"
	namedStyleHeading6   = "HEADING_6"
)

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

type Options struct {
	FontFamily     string
	FontSize       float64
	TextColor      string
	Background     string
	Link           string
	ClearLink      bool
	ResolvedLink   *docs.Link
	Code           bool
	Bold           bool
	ClearBold      bool
	Italic         bool
	ClearItalic    bool
	Underline      bool
	ClearUnderline bool
	Strikethrough  bool
	ClearStrike    bool
	Alignment      string
	LineSpacing    float64
	HeadingLevel   *int
	NamedStyle     string
}

func (o Options) Any() bool {
	return strings.TrimSpace(o.FontFamily) != "" ||
		o.FontSize != 0 ||
		strings.TrimSpace(o.TextColor) != "" ||
		strings.TrimSpace(o.Background) != "" ||
		strings.TrimSpace(o.Link) != "" ||
		o.ClearLink ||
		o.Code ||
		o.Bold || o.ClearBold ||
		o.Italic || o.ClearItalic ||
		o.Underline || o.ClearUnderline ||
		o.Strikethrough || o.ClearStrike ||
		strings.TrimSpace(o.Alignment) != "" ||
		o.LineSpacing != 0 ||
		o.HeadingLevel != nil ||
		strings.TrimSpace(o.NamedStyle) != ""
}

func BuildRequests(options Options, start, end int64, tabID string) ([]*docs.Request, error) {
	if start <= 0 || end <= start {
		return nil, invalidf("invalid format range: %d..%d", start, end)
	}

	textReq, hasText, err := buildTextStyleRequest(options, start, end, tabID)
	if err != nil {
		return nil, err
	}

	paragraphReq, hasParagraph, err := buildParagraphStyleRequest(options, start, end, tabID)
	if err != nil {
		return nil, err
	}

	requests := make([]*docs.Request, 0, 2)
	if hasText {
		requests = append(requests, textReq)
	}

	if hasParagraph {
		requests = append(requests, paragraphReq)
	}

	if len(requests) == 0 {
		return nil, ValidationError("no formatting flags provided")
	}

	return requests, nil
}

func buildTextStyleRequest(options Options, start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.TextStyle{}
	var fields []string

	if options.Code {
		if strings.TrimSpace(options.FontFamily) != "" {
			return nil, false, ValidationError("--code cannot be combined with --font-family")
		}

		if strings.TrimSpace(options.Background) != "" {
			return nil, false, ValidationError("--code cannot be combined with --bg-color")
		}
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: "Courier New"}
		style.BackgroundColor = greyColor(codeBackgroundGrey)

		fields = append(fields, "weightedFontFamily", "backgroundColor")
	}

	if font := strings.TrimSpace(options.FontFamily); font != "" {
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: font}

		fields = append(fields, "weightedFontFamily")
	}

	if options.FontSize < 0 {
		return nil, false, ValidationError("--font-size must be positive")
	}

	if options.FontSize > 0 {
		style.FontSize = &docs.Dimension{Magnitude: options.FontSize, Unit: "PT"}

		fields = append(fields, "fontSize")
	}

	if color := strings.TrimSpace(options.TextColor); color != "" {
		optionalColor, err := Color(color, "--text-color")
		if err != nil {
			return nil, false, err
		}
		style.ForegroundColor = optionalColor

		fields = append(fields, "foregroundColor")
	}

	if color := strings.TrimSpace(options.Background); color != "" {
		optionalColor, err := Color(color, "--bg-color")
		if err != nil {
			return nil, false, err
		}
		style.BackgroundColor = optionalColor

		fields = append(fields, "backgroundColor")
	}

	if strings.TrimSpace(options.Link) != "" && options.ClearLink {
		return nil, false, ValidationError("--link and --no-link cannot be combined")
	}

	if link := strings.TrimSpace(options.Link); link != "" {
		resolved := options.ResolvedLink
		if resolved == nil {
			var err error

			resolved, err = formatLink(link)
			if err != nil {
				return nil, false, err
			}
		}
		style.Link = resolved

		fields = append(fields, "link")
	}

	if options.ClearLink {
		style.NullFields = append(style.NullFields, "Link")
		fields = append(fields, "link")
	}

	addBoolStyle := func(set, unset bool, field, forceField string, apply func(bool)) error {
		if set && unset {
			return invalidf("--%s and --no-%s cannot be combined", field, field)
		}

		if set || unset {
			apply(set)

			fields = append(fields, field)
			if unset {
				style.ForceSendFields = append(style.ForceSendFields, forceField)
			}
		}

		return nil
	}
	if err := addBoolStyle(options.Bold, options.ClearBold, "bold", "Bold", func(v bool) { style.Bold = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Italic, options.ClearItalic, "italic", "Italic", func(v bool) { style.Italic = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Underline, options.ClearUnderline, "underline", "Underline", func(v bool) { style.Underline = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Strikethrough, options.ClearStrike, "strikethrough", "Strikethrough", func(v bool) { style.Strikethrough = v }); err != nil {
		return nil, false, err
	}

	if len(fields) == 0 {
		return nil, false, nil
	}

	return &docs.Request{UpdateTextStyle: &docs.UpdateTextStyleRequest{
		Range:     &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		TextStyle: style,
		Fields:    strings.Join(fields, ","),
	}}, true, nil
}

func buildParagraphStyleRequest(options Options, start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.ParagraphStyle{}
	var fields []string

	if align := strings.TrimSpace(options.Alignment); align != "" {
		resolved, err := formatAlignment(align)
		if err != nil {
			return nil, false, err
		}
		style.Alignment = resolved

		fields = append(fields, "alignment")
	}

	if options.LineSpacing < 0 {
		return nil, false, ValidationError("--line-spacing must be positive")
	}

	if options.LineSpacing > 0 {
		style.LineSpacing = options.LineSpacing

		fields = append(fields, "lineSpacing")
	}

	namedStyle, err := formatNamedStyle(options.HeadingLevel, options.NamedStyle)
	if err != nil {
		return nil, false, err
	}

	if namedStyle != "" {
		style.NamedStyleType = namedStyle

		fields = append(fields, "namedStyleType")
	}

	if len(fields) == 0 {
		return nil, false, nil
	}

	return &docs.Request{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
		Range:          &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		ParagraphStyle: style,
		Fields:         strings.Join(fields, ","),
	}}, true, nil
}

func formatLink(value string) (*docs.Link, error) {
	link := strings.TrimSpace(value)
	if link == "" {
		return nil, ValidationError("--link target cannot be empty")
	}

	if target, ok := strings.CutPrefix(link, "#"); ok {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, ValidationError("--link target cannot be empty")
		}

		return &docs.Link{BookmarkId: target}, nil
	}

	return &docs.Link{Url: link}, nil
}

func formatNamedStyle(headingLevel *int, namedStyle string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(namedStyle))
	if headingLevel != nil && trimmed != "" {
		return "", ValidationError("--heading-level and --named-style cannot be combined")
	}

	if headingLevel != nil {
		if *headingLevel < 1 || *headingLevel > 6 {
			return "", ValidationError("--heading-level must be between 1 and 6")
		}

		return fmt.Sprintf("HEADING_%d", *headingLevel), nil
	}

	if trimmed == "" {
		return "", nil
	}

	switch trimmed {
	case namedStyleNormalText, namedStyleTitle, namedStyleSubtitle,
		namedStyleHeading1, namedStyleHeading2, namedStyleHeading3,
		namedStyleHeading4, namedStyleHeading5, namedStyleHeading6:
		return trimmed, nil
	default:
		return "", ValidationError("--named-style must be one of NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6")
	}
}

// Color parses a Docs color flag in #RRGGBB or #RGB form.
func Color(hex, flag string) (*docs.OptionalColor, error) {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return nil, invalidf("%s must be #RRGGBB or #RGB", flag)
	}

	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}}}, nil
}

func formatAlignment(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "start":
		return "START", nil
	case "center", "centre":
		return "CENTER", nil
	case "right", "end":
		return "END", nil
	case "justify", "justified":
		return "JUSTIFIED", nil
	default:
		return "", ValidationError("--alignment must be left, center, right, justify, start, end, or justified")
	}
}

func greyColor(intensity float64) *docs.OptionalColor {
	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{
		Red: intensity, Green: intensity, Blue: intensity,
	}}}
}

func parseHexColor(hex string) (r, g, b float64, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}

	if len(hex) != 6 {
		return 0, 0, 0, false
	}

	rgb, err := strconv.ParseUint(hex, 16, 24)
	if err != nil {
		return 0, 0, 0, false
	}

	return float64((rgb>>16)&0xFF) / 255.0, float64((rgb>>8)&0xFF) / 255.0, float64(rgb&0xFF) / 255.0, true
}

func invalidf(format string, args ...any) error {
	return ValidationError(fmt.Sprintf(format, args...))
}
