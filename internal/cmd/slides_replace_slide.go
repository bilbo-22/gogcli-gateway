package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesReplaceSlideCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID to replace"`
	Image          string  `arg:"" name:"image" help:"Local image file (PNG/JPG/GIF)" type:"existingfile"`
	Notes          *string `name:"notes" help:"New speaker notes text (omit to preserve existing notes; use --notes '' to clear)"`
	NotesFile      string  `name:"notes-file" help:"Path to file containing new speaker notes" type:"existingfile"`
}

func (c *SlidesReplaceSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	notes, updateNotes, err := resolveSlidesNotesInput(c.Notes, c.NotesFile)
	if err != nil {
		return err
	}

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}

	// Validate image format.
	ext := strings.ToLower(filepath.Ext(c.Image))
	var mimeType string
	switch ext {
	case extPNG:
		mimeType = mimePNG
	case imageExtJPG, imageExtJPEG:
		mimeType = imageMimeJPEG
	case imageExtGIF:
		mimeType = imageMimeGIF
	default:
		return usagef("unsupported image format %q (use PNG, JPG, or GIF)", ext)
	}

	if dryRunErr := dryRunExit(ctx, flags, "slides.replace-slide", map[string]any{
		"presentation_id": presentationID,
		"slide_id":        slideID,
		"image":           c.Image,
		"mime_type":       mimeType,
		"update_notes":    updateNotes,
		"notes":           updateNotes && notes != "",
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	driveSvc, err := driveService(ctx, account)
	if err != nil {
		return err
	}

	// Get presentation to find the slide and its image element.
	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	var imageObjectID string
	slide, slideIndex := findSlidesPageByID(pres, slideID)
	if slide != nil {
		for _, el := range slide.PageElements {
			if el != nil && el.Image != nil {
				imageObjectID = el.ObjectId
				break
			}
		}
	}
	if slideIndex == -1 {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}
	if imageObjectID == "" {
		return fmt.Errorf("no image found on slide %s", slideID)
	}

	// Upload new image to Drive.
	imgFile, err := os.Open(c.Image)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer imgFile.Close()

	driveFile, err := driveSvc.Files.Create(&drive.File{
		Name:     filepath.Base(c.Image),
		MimeType: mimeType,
	}).Media(imgFile).Fields("id, webContentLink").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("upload image to Drive: %w", err)
	}

	// Clean up the temporary Drive file when done.
	defer func() {
		_ = driveSvc.Files.Delete(driveFile.Id).Context(ctx).Do()
	}()

	// Make publicly readable so the Slides API can fetch it.
	_, err = driveSvc.Permissions.Create(driveFile.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set image permissions: %w", err)
	}

	imageURL := driveImageDownloadURL(driveFile.Id)

	// Replace the image in-place.
	requests := []*slides.Request{
		{
			ReplaceImage: &slides.ReplaceImageRequest{
				ImageObjectId:      imageObjectID,
				ImageReplaceMethod: "CENTER_CROP",
				Url:                imageURL,
			},
		},
	}

	// Optionally update notes in the same batch.
	if updateNotes {
		notesObjectID := findSpeakerNotesObjectID(slide)
		if notesObjectID == "" {
			return fmt.Errorf("could not find speaker notes placeholder on slide %s", slideID)
		}
		notesPage := slide.SlideProperties.NotesPage
		requests = append(requests, buildSlidesReplaceTextRequests(notesObjectID, notes, slidesPageElementHasText(notesPage, notesObjectID))...)
	}

	err = batchUpdateSlidesImageRequests(ctx, slidesSvc, presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	})
	if err != nil {
		return fmt.Errorf("replace slide image: %w", err)
	}

	link := fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presentationID)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"slideNumber":    slideIndex + 1,
			"slideObjectId":  slideID,
			"presentationId": presentationID,
			"link":           link,
		})
	}

	u.Out().Linef("Replaced image on slide %d (%s)", slideIndex+1, slideID)
	if updateNotes {
		u.Out().Linef("Updated speaker notes")
	}
	u.Out().Linef("link\t%s", link)
	return nil
}
