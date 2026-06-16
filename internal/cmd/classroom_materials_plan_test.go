package cmd

import (
	"strings"
	"testing"
)

func TestBuildClassroomMaterialCreatePlan(t *testing.T) {
	plan, err := buildClassroomMaterialCreatePlan(classroomMaterialInput{
		CourseID:    " c1 ",
		Title:       " Reading ",
		Description: " Chapter 1 ",
		State:       "draft",
		Scheduled:   "2024-03-10T12:00:00Z",
		TopicID:     " t1 ",
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.CourseID != "c1" {
		t.Fatalf("course ID = %q", plan.CourseID)
	}
	material := plan.Material
	if material.Title != "Reading" || material.Description != "Chapter 1" ||
		material.State != "DRAFT" || material.ScheduledTime != "2024-03-10T12:00:00Z" ||
		material.TopicId != "t1" {
		t.Fatalf("unexpected material: %#v", material)
	}
}

func TestBuildClassroomMaterialUpdatePlan(t *testing.T) {
	plan, err := buildClassroomMaterialUpdatePlan(classroomMaterialUpdateInput{
		classroomMaterialInput: classroomMaterialInput{
			CourseID:    " c1 ",
			Title:       " Updated ",
			Description: " New description ",
			State:       "published",
			Scheduled:   "2024-03-11T12:00:00Z",
			TopicID:     " t2 ",
		},
		MaterialID: " m1 ",
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.CourseID != "c1" || plan.MaterialID != "m1" {
		t.Fatalf("unexpected identifiers: %#v", plan)
	}
	if plan.UpdateMask != "title,description,state,scheduledTime,topicId" {
		t.Fatalf("update mask = %q", plan.UpdateMask)
	}
	if strings.Join(plan.UpdateFields, ",") != plan.UpdateMask {
		t.Fatalf("fields = %#v, mask = %q", plan.UpdateFields, plan.UpdateMask)
	}
	if plan.Material.Title != "Updated" || plan.Material.State != "PUBLISHED" ||
		plan.Material.TopicId != "t2" {
		t.Fatalf("unexpected material: %#v", plan.Material)
	}
}

func TestBuildClassroomMaterialPlanValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "create course ID",
			run: func() error {
				_, err := buildClassroomMaterialCreatePlan(classroomMaterialInput{Title: "Reading"})
				return err
			},
			want: "empty courseId",
		},
		{
			name: "create title",
			run: func() error {
				_, err := buildClassroomMaterialCreatePlan(classroomMaterialInput{CourseID: "c1"})
				return err
			},
			want: "empty title",
		},
		{
			name: "update material ID",
			run: func() error {
				_, err := buildClassroomMaterialUpdatePlan(classroomMaterialUpdateInput{
					classroomMaterialInput: classroomMaterialInput{CourseID: "c1"},
				})
				return err
			},
			want: "empty materialId",
		},
		{
			name: "no updates",
			run: func() error {
				_, err := buildClassroomMaterialUpdatePlan(classroomMaterialUpdateInput{
					classroomMaterialInput: classroomMaterialInput{CourseID: "c1"},
					MaterialID:             "m1",
				})
				return err
			},
			want: "no updates specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
