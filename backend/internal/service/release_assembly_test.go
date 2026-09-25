package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
)

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Now().UTC()
}

func TestAuthorizationApprovalBlockedBySuspendedSubpart(t *testing.T) {
	db := newAssemblyTestDB(t)
	ctx := context.Background()

	partRepo := repository.NewAircraftPartRepository(db)
	assemblyRepo := repository.NewPartAssemblyRepository(db)
	assemblyService := NewPartAssemblyService(assemblyRepo, partRepo, nil)

	component := seedPart(t, db, "REL-COMP", "released")
	blockedPart := seedPart(t, db, "REL-SUB-HOLD", "hold")
	if _, err := assemblyService.Register(ctx, dto.RegisterAssembly{ParentCode: component.Code, ChildCode: blockedPart.Code}, "operator", "link"); err != nil {
		t.Fatalf("register assembly: %v", err)
	}

	authRepo := repository.NewReleaseAuthorizationRepository(db)
	authService := NewReleaseAuthorizationService(authRepo, nil)
	authService.SetPartChecker(partRepo)
	authService.SetAssemblyReleaseVerifier(assemblyService)

	partID := component.ID
	created, err := authService.Create(ctx, dto.CreateReleaseAuthorization{
		Code: "AUTH-BLOCK-01", Name: "Blocked release", Facility: "Hangar 2", Owner: "Release desk",
		Category: "engine", RiskLevel: "high", EffectiveAt: mustTime(t), Evidence: "evidence",
		PartID: &partID, RelatedCode: component.Code,
	}, "operator", "auth-create")
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}
	review, err := authService.Transition(ctx, created.ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: created.Version, Reason: "submit",
	}, "operator", model.RoleOperator, "auth-review")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err = authService.Transition(ctx, review.ID, dto.TransitionRequest{
		Status: "approved", ExpectedVersion: review.Version, Reason: "approve",
	}, "reviewer", model.RoleReviewer, "auth-approve-blocked")
	var blockedErr *AssemblyBlockedError
	if !errors.As(err, &blockedErr) {
		t.Fatalf("approval must be blocked by suspended subpart, got %v", err)
	}
	if len(blockedErr.Blocked) != 1 || blockedErr.Blocked[0].Code != "REL-SUB-HOLD" {
		t.Fatalf("blocked list must carry the subpart code, got %#v", blockedErr.Blocked)
	}

	// 授权必须仍然留在待复核。
	stuck, err := authService.Get(ctx, review.ID)
	if err != nil {
		t.Fatalf("reload authorization: %v", err)
	}
	if stuck.Status != "review" || stuck.Version != 2 {
		t.Fatalf("authorization must remain in review at version 2, got status=%s version=%d", stuck.Status, stuck.Version)
	}

	// 子件放行后复核员可以批准。
	if err := db.Model(&model.AircraftPart{}).Where("id = ?", blockedPart.ID).Update("status", "released").Error; err != nil {
		t.Fatalf("release subpart: %v", err)
	}
	approved, err := authService.Transition(ctx, review.ID, dto.TransitionRequest{
		Status: "approved", ExpectedVersion: review.Version, Reason: "subparts released",
	}, "reviewer", model.RoleReviewer, "auth-approve-ok")
	if err != nil {
		t.Fatalf("approval must pass after subpart release, got %v", err)
	}
	if approved.Status != "approved" || approved.Version != 3 {
		t.Fatalf("unexpected approved authorization: status=%s version=%d", approved.Status, approved.Version)
	}
}

func TestAuthorizationWithoutPartSkipsAssemblyVerification(t *testing.T) {
	db := newAssemblyTestDB(t)
	authService := NewReleaseAuthorizationService(repository.NewReleaseAuthorizationRepository(db), nil)

	created, err := authService.Create(context.Background(), authorizationInput("AUTH-NOPART-01"), "operator", "create")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	review, err := authService.Transition(context.Background(), created.ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: 1, Reason: "submit",
	}, "operator", model.RoleOperator, "review")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := authService.Transition(context.Background(), review.ID, dto.TransitionRequest{
		Status: "approved", ExpectedVersion: 2, Reason: "no linked component",
	}, "reviewer", model.RoleReviewer, "approve"); err != nil {
		t.Fatalf("authorization without linked part must be approvable, got %v", err)
	}
}
