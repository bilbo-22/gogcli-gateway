package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type infoViaDriveOptions struct {
	ArgName      string
	ExpectedMime string
	KindLabel    string
}

const infoViaDriveDefaultKindLabel = "expected type"

func infoViaDrive(ctx context.Context, flags *RootFlags, opts infoViaDriveOptions, id string) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = normalizeGoogleID(strings.TrimSpace(id))
	if id == "" {
		return usage(fmt.Sprintf("empty %s", argName))
	}

	svc, err := driveService(ctx, account)
	if err != nil {
		return err
	}

	f, err := svc.Files.Get(id).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, webViewLink, parents").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if f == nil {
		return errors.New("file not found")
	}
	if opts.ExpectedMime != "" && f.MimeType != opts.ExpectedMime {
		label := strings.TrimSpace(opts.KindLabel)
		if label == "" {
			label = infoViaDriveDefaultKindLabel
		}
		return fmt.Errorf("file is not a %s (mimeType=%q)", label, f.MimeType)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{strFile: f})
	}

	u.Out().Linef("id\t%s", f.Id)
	u.Out().Linef("name\t%s", f.Name)
	u.Out().Linef("mime\t%s", f.MimeType)
	if f.WebViewLink != "" {
		u.Out().Linef("link\t%s", f.WebViewLink)
	}
	if f.CreatedTime != "" {
		u.Out().Linef("created\t%s", f.CreatedTime)
	}
	if f.ModifiedTime != "" {
		u.Out().Linef("modified\t%s", f.ModifiedTime)
	}
	if len(f.Parents) > 0 {
		u.Out().Linef("parents\t%s", strings.Join(f.Parents, ","))
	}
	return nil
}
