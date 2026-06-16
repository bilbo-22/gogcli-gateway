package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ClassroomAnnouncementsCmd struct {
	List      ClassroomAnnouncementsListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List announcements"`
	Get       ClassroomAnnouncementsGetCmd       `cmd:"" aliases:"info,show" help:"Get an announcement"`
	Create    ClassroomAnnouncementsCreateCmd    `cmd:"" aliases:"add,new" help:"Create an announcement"`
	Update    ClassroomAnnouncementsUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update an announcement"`
	Delete    ClassroomAnnouncementsDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete an announcement"`
	Assignees ClassroomAnnouncementsAssigneesCmd `cmd:"" name:"assignees" aliases:"assign" help:"Modify announcement assignees"`
}

type ClassroomAnnouncementsListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	States    string `name:"state" help:"Announcement states filter (comma-separated: DRAFT,PUBLISHED,DELETED)"`
	OrderBy   string `name:"order-by" help:"Order by (e.g., updateTime desc)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomAnnouncementsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	fetch := func(pageToken string) ([]*classroom.Announcement, string, error) {
		call := svc.Courses.Announcements.List(courseID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.AnnouncementStates(upper...)
		}
		if v := strings.TrimSpace(c.OrderBy); v != "" {
			call.OrderBy(v)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", wrapClassroomError(callErr)
		}
		return resp.Announcements, resp.NextPageToken, nil
	}

	announcements, nextPageToken, err := fetchClassroomPagedList(c.All, c.Page, fetch)
	if err != nil {
		return err
	}

	return writeClassroomPagedList(
		ctx,
		"announcements",
		announcements,
		nextPageToken,
		"No announcements",
		c.FailEmpty,
		false,
		classroomAnnouncementColumns(),
	)
}

type ClassroomAnnouncementsGetCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	ann, err := svc.Courses.Announcements.Get(courseID, announcementID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": ann})
	}

	u.Out().Linef("id\t%s", ann.Id)
	u.Out().Linef("state\t%s", ann.State)
	if ann.Text != "" {
		u.Out().Linef("text\t%s", ann.Text)
	}
	if ann.ScheduledTime != "" {
		u.Out().Linef("scheduled\t%s", ann.ScheduledTime)
	}
	if ann.AlternateLink != "" {
		u.Out().Linef("link\t%s", ann.AlternateLink)
	}
	return nil
}

type ClassroomAnnouncementsCreateCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Text      string `name:"text" help:"Announcement text" required:""`
	State     string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
}

func (c *ClassroomAnnouncementsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomAnnouncementCreatePlan(classroomAnnouncementInput{
		CourseID:  c.CourseID,
		Text:      c.Text,
		State:     c.State,
		Scheduled: c.Scheduled,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.create", map[string]any{
		"course_id":    plan.CourseID,
		"announcement": plan.Announcement,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.Announcements.Create(plan.CourseID, plan.Announcement).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("state\t%s", created.State)
	return nil
}

type ClassroomAnnouncementsUpdateCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
	Text           string `name:"text" help:"Announcement text"`
	State          string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled      string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
}

func (c *ClassroomAnnouncementsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomAnnouncementUpdatePlan(classroomAnnouncementUpdateInput{
		classroomAnnouncementInput: classroomAnnouncementInput{
			CourseID:  c.CourseID,
			Text:      c.Text,
			State:     c.State,
			Scheduled: c.Scheduled,
		},
		AnnouncementID: c.AnnouncementID,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.update", map[string]any{
		"course_id":       plan.CourseID,
		"announcement_id": plan.AnnouncementID,
		"update_mask":     plan.UpdateMask,
		"update_fields":   plan.UpdateFields,
		"announcement":    plan.Announcement,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.Patch(plan.CourseID, plan.AnnouncementID, plan.Announcement).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("state\t%s", updated.State)
	return nil
}

type ClassroomAnnouncementsDeleteCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "classroom.announcements.delete", map[string]any{
		"course_id":       courseID,
		"announcement_id": announcementID,
	}, fmt.Sprintf("delete announcement %s from %s", announcementID, courseID)); err != nil {
		return err
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Courses.Announcements.Delete(courseID, announcementID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("announcementId", announcementID),
	)
}

type ClassroomAnnouncementsAssigneesCmd struct {
	CourseID       string   `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string   `arg:"" name:"announcementId" help:"Announcement ID"`
	Mode           string   `name:"mode" help:"Assignee mode: ALL_STUDENTS, INDIVIDUAL_STUDENTS"`
	AddStudents    []string `name:"add-student" help:"Student IDs to add" sep:","`
	RemoveStudents []string `name:"remove-student" help:"Student IDs to remove" sep:","`
}

func (c *ClassroomAnnouncementsAssigneesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	mode, opts, err := normalizeAssigneeMode(c.Mode, c.AddStudents, c.RemoveStudents)
	if err != nil {
		return usage(err.Error())
	}
	req := &classroom.ModifyAnnouncementAssigneesRequest{
		AssigneeMode:                    mode,
		ModifyIndividualStudentsOptions: opts,
	}
	if req.AssigneeMode == "" && req.ModifyIndividualStudentsOptions == nil {
		return usage("no assignee changes specified")
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.assignees", map[string]any{
		"course_id":       courseID,
		"announcement_id": announcementID,
		"request":         req,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.ModifyAssignees(courseID, announcementID, req).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("assignee_mode\t%s", updated.AssigneeMode)
	return nil
}

func truncateClassroomText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" || maxLen <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}
