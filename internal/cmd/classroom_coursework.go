package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ClassroomCourseworkCmd struct {
	List      ClassroomCourseworkListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List coursework"`
	Get       ClassroomCourseworkGetCmd       `cmd:"" aliases:"info,show" help:"Get coursework"`
	Create    ClassroomCourseworkCreateCmd    `cmd:"" aliases:"add,new" help:"Create coursework"`
	Update    ClassroomCourseworkUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update coursework"`
	Delete    ClassroomCourseworkDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete coursework"`
	Assignees ClassroomCourseworkAssigneesCmd `cmd:"" name:"assignees" aliases:"assign" help:"Modify coursework assignees"`
}

type ClassroomCourseworkListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	States    string `name:"state" help:"Coursework states filter (comma-separated: DRAFT,PUBLISHED,DELETED)"`
	Topic     string `name:"topic" help:"Filter by topic ID"`
	OrderBy   string `name:"order-by" help:"Order by (e.g., updateTime desc, dueDate desc)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	ScanPages int    `name:"scan-pages" help:"Pages to scan when filtering by topic" default:"3"`
}

func (c *ClassroomCourseworkListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	makeCall := func(page string) (*classroom.ListCourseWorkResponse, error) {
		call := svc.Courses.CourseWork.List(courseID).PageSize(c.Max).PageToken(page).Context(ctx)
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.CourseWorkStates(upper...)
		}
		if v := strings.TrimSpace(c.OrderBy); v != "" {
			call.OrderBy(v)
		}
		return call.Do()
	}

	fetch := func(page string) ([]*classroom.CourseWork, string, error) {
		resp, callErr := makeCall(page)
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.CourseWork, resp.NextPageToken, nil
	}

	var coursework []*classroom.CourseWork
	var nextPageToken string
	if c.All {
		all, _, err := loadPagedItems(c.Page, true, fetch)
		if err != nil {
			return wrapClassroomError(err)
		}
		all = nonNilClassroomItems(all)
		coursework = all
		if topic := strings.TrimSpace(c.Topic); topic != "" {
			filtered := coursework[:0]
			for _, work := range coursework {
				if work == nil {
					continue
				}
				if work.TopicId == topic {
					filtered = append(filtered, work)
				}
			}
			coursework = filtered
		}
	} else {
		var err error
		coursework, nextPageToken, err = scanClassroomTopicPages(
			c.Topic,
			c.Page,
			c.ScanPages,
			fetch,
			func(work *classroom.CourseWork) string {
				if work == nil {
					return ""
				}
				return work.TopicId
			},
		)
		if err != nil {
			return wrapClassroomError(err)
		}
	}
	coursework = nonNilClassroomItems(coursework)

	return writeClassroomPagedList(
		ctx,
		"coursework",
		coursework,
		nextPageToken,
		"No coursework",
		c.FailEmpty,
		true,
		classroomCourseworkColumns(),
	)
}

type ClassroomCourseworkGetCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
}

func (c *ClassroomCourseworkGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	work, err := svc.Courses.CourseWork.Get(courseID, courseworkID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"coursework": work})
	}

	u.Out().Linef("id\t%s", work.Id)
	u.Out().Linef("title\t%s", work.Title)
	if work.Description != "" {
		u.Out().Linef("description\t%s", work.Description)
	}
	u.Out().Linef("state\t%s", work.State)
	u.Out().Linef("type\t%s", work.WorkType)
	if due := formatClassroomDue(work.DueDate, work.DueTime); due != "" {
		u.Out().Linef("due\t%s", due)
	}
	if work.ScheduledTime != "" {
		u.Out().Linef("scheduled\t%s", work.ScheduledTime)
	}
	if work.TopicId != "" {
		u.Out().Linef("topic_id\t%s", work.TopicId)
	}
	if work.MaxPoints != 0 {
		u.Out().Linef("max_points\t%s", formatFloatValue(work.MaxPoints))
	}
	if work.AlternateLink != "" {
		u.Out().Linef("link\t%s", work.AlternateLink)
	}
	return nil
}

type ClassroomCourseworkCreateCmd struct {
	CourseID    string  `arg:"" name:"courseId" help:"Course ID or alias"`
	Title       string  `name:"title" help:"Title" required:""`
	Description string  `name:"description" help:"Description"`
	WorkType    string  `name:"type" help:"Work type: ASSIGNMENT, SHORT_ANSWER_QUESTION, MULTIPLE_CHOICE_QUESTION" default:"ASSIGNMENT"`
	State       string  `name:"state" help:"State: PUBLISHED, DRAFT"`
	MaxPoints   float64 `name:"max-points" help:"Max points"`
	Due         string  `name:"due" help:"Due date/time (RFC3339 or YYYY-MM-DD [HH:MM])"`
	DueDate     string  `name:"due-date" help:"Due date (YYYY-MM-DD)"`
	DueTime     string  `name:"due-time" help:"Due time (HH:MM or HH:MM:SS)"`
	Scheduled   string  `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID     string  `name:"topic" help:"Topic ID"`
}

func (c *ClassroomCourseworkCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomCourseworkCreatePlan(classroomCourseworkCreateInput{
		classroomCourseworkInput: classroomCourseworkInput{
			CourseID:    c.CourseID,
			Title:       c.Title,
			Description: c.Description,
			State:       c.State,
			MaxPoints:   c.MaxPoints,
			Due:         c.Due,
			DueDate:     c.DueDate,
			DueTime:     c.DueTime,
			Scheduled:   c.Scheduled,
			TopicID:     c.TopicID,
		},
		WorkType: c.WorkType,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.create", map[string]any{
		"course_id":  plan.CourseID,
		"coursework": plan.Coursework,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.CourseWork.Create(plan.CourseID, plan.Coursework).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"coursework": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("title\t%s", created.Title)
	u.Out().Linef("state\t%s", created.State)
	return nil
}

type ClassroomCourseworkUpdateCmd struct {
	CourseID     string  `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string  `arg:"" name:"courseworkId" help:"Coursework ID"`
	Title        string  `name:"title" help:"Title"`
	Description  string  `name:"description" help:"Description"`
	State        string  `name:"state" help:"State: PUBLISHED, DRAFT"`
	MaxPoints    float64 `name:"max-points" help:"Max points"`
	Due          string  `name:"due" help:"Due date/time (RFC3339 or YYYY-MM-DD [HH:MM])"`
	DueDate      string  `name:"due-date" help:"Due date (YYYY-MM-DD)"`
	DueTime      string  `name:"due-time" help:"Due time (HH:MM or HH:MM:SS)"`
	Scheduled    string  `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID      string  `name:"topic" help:"Topic ID"`
}

func (c *ClassroomCourseworkUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomCourseworkUpdatePlan(classroomCourseworkUpdateInput{
		classroomCourseworkInput: classroomCourseworkInput{
			CourseID:    c.CourseID,
			Title:       c.Title,
			Description: c.Description,
			State:       c.State,
			MaxPoints:   c.MaxPoints,
			Due:         c.Due,
			DueDate:     c.DueDate,
			DueTime:     c.DueTime,
			Scheduled:   c.Scheduled,
			TopicID:     c.TopicID,
		},
		CourseworkID: c.CourseworkID,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.update", map[string]any{
		"course_id":     plan.CourseID,
		"coursework_id": plan.CourseworkID,
		"update_mask":   plan.UpdateMask,
		"update_fields": plan.UpdateFields,
		"coursework":    plan.Coursework,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWork.Patch(plan.CourseID, plan.CourseworkID, plan.Coursework).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"coursework": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("title\t%s", updated.Title)
	u.Out().Linef("state\t%s", updated.State)
	return nil
}

type ClassroomCourseworkDeleteCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
}

func (c *ClassroomCourseworkDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "classroom.coursework.delete", map[string]any{
		"course_id":     courseID,
		"coursework_id": courseworkID,
	}, fmt.Sprintf("delete coursework %s from %s", courseworkID, courseID)); err != nil {
		return err
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Courses.CourseWork.Delete(courseID, courseworkID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("courseworkId", courseworkID),
	)
}

type ClassroomCourseworkAssigneesCmd struct {
	CourseID       string   `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID   string   `arg:"" name:"courseworkId" help:"Coursework ID"`
	Mode           string   `name:"mode" help:"Assignee mode: ALL_STUDENTS, INDIVIDUAL_STUDENTS"`
	AddStudents    []string `name:"add-student" help:"Student IDs to add" sep:","`
	RemoveStudents []string `name:"remove-student" help:"Student IDs to remove" sep:","`
}

func (c *ClassroomCourseworkAssigneesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	mode, opts, err := normalizeAssigneeMode(c.Mode, c.AddStudents, c.RemoveStudents)
	if err != nil {
		return usage(err.Error())
	}
	req := &classroom.ModifyCourseWorkAssigneesRequest{
		AssigneeMode:                    mode,
		ModifyIndividualStudentsOptions: opts,
	}
	if req.AssigneeMode == "" && req.ModifyIndividualStudentsOptions == nil {
		return usage("no assignee changes specified")
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.assignees", map[string]any{
		"course_id":     courseID,
		"coursework_id": courseworkID,
		"request":       req,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWork.ModifyAssignees(courseID, courseworkID, req).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"coursework": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("assignee_mode\t%s", updated.AssigneeMode)
	return nil
}
