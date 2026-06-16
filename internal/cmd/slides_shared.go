package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/slides/v1"
)

const (
	imageExtJPG   = ".jpg"
	imageExtJPEG  = ".jpeg"
	imageExtGIF   = ".gif"
	imageMimeJPEG = "image/jpeg"
	imageMimeGIF  = "image/gif"

	placeholderTypeBody = "BODY"
)

func resolveSlidesNotesInput(notes *string, notesFile string) (string, bool, error) {
	if notesFile != "" {
		// The caller's Kong field uses type:"existingfile"; this helper only
		// centralizes the read for the two Slides note commands.
		//nolint:gosec
		data, err := os.ReadFile(notesFile)
		if err != nil {
			return "", false, fmt.Errorf("read notes file: %w", err)
		}
		return string(data), true, nil
	}
	if notes != nil {
		return *notes, true, nil
	}
	return "", false, nil
}

func findSlidesPageByID(pres *slides.Presentation, slideID string) (*slides.Page, int) {
	if pres == nil {
		return nil, -1
	}
	for i, slide := range pres.Slides {
		if slide != nil && slide.ObjectId == slideID {
			return slide, i
		}
	}
	return nil, -1
}

func findSpeakerNotesObjectID(slide *slides.Page) string {
	if slide == nil || slide.SlideProperties == nil || slide.SlideProperties.NotesPage == nil {
		return ""
	}

	notesPage := slide.SlideProperties.NotesPage
	if notesPage.NotesProperties != nil && notesPage.NotesProperties.SpeakerNotesObjectId != "" {
		return notesPage.NotesProperties.SpeakerNotesObjectId
	}

	for _, el := range notesPage.PageElements {
		if el != nil && el.Shape != nil && el.Shape.Placeholder != nil &&
			el.Shape.Placeholder.Type == placeholderTypeBody {
			return el.ObjectId
		}
	}
	return ""
}

func slidesPageElementHasText(page *slides.Page, objectID string) bool {
	if page == nil || objectID == "" {
		return false
	}
	for _, el := range page.PageElements {
		if el == nil || el.ObjectId != objectID || el.Shape == nil || el.Shape.Text == nil {
			continue
		}
		for _, textElement := range el.Shape.Text.TextElements {
			if textElement != nil && textElement.TextRun != nil && textElement.TextRun.Content != "" {
				return true
			}
		}
	}
	return false
}

func buildSlidesReplaceTextRequests(objectID string, text string, hasExistingText bool) []*slides.Request {
	requests := []*slides.Request{}
	if hasExistingText {
		requests = append(requests, &slides.Request{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId: objectID,
				TextRange: &slides.Range{
					Type: "ALL",
				},
			},
		})
	}
	if text != "" {
		requests = append(requests, &slides.Request{
			InsertText: &slides.InsertTextRequest{
				ObjectId: objectID,
				Text:     text,
			},
		})
	}
	return requests
}

func buildSlidesClearAndInsertTextRequests(objectID string, text string) []*slides.Request {
	return buildSlidesReplaceTextRequests(objectID, text, true)
}

func batchUpdateSlidesImageRequests(ctx context.Context, svc *slides.Service, presentationID string, req *slides.BatchUpdatePresentationRequest) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		_, err := svc.Presentations.BatchUpdate(presentationID, req).Context(ctx).Do()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt >= len(docsImageInsertRetryDelays) || !isRetryableSlidesImageRequestError(err) {
			return lastErr
		}
		if err := waitDocsImageInsertRetry(ctx, docsImageInsertRetryDelays[attempt]); err != nil {
			return err
		}
	}
}

func isRetryableSlidesImageRequestError(err error) bool {
	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code >= 500 {
			return true
		}
		if apiErr.Code == 400 && strings.Contains(apiErr.Message, "retrieving the image") {
			return true
		}
	}
	errStr := err.Error()
	return strings.Contains(errStr, "retrieving the image") ||
		strings.Contains(errStr, "provided image should be publicly accessible")
}
