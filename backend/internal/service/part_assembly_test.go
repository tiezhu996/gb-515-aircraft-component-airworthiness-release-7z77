package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newAssemblyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.AuditLog{}, &model.AircraftPart{}, &model.PartAssembly{},
		&model.ReleaseAuthorization{}, &model.ReleaseAuthorizationRevision{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createPartForAssembly(t *testing.T, db *gorm.DB, code, status string) *model.AircraftPart {
	t.Helper()
	part := &model.AircraftPart{
		BaseModel: model.BaseModel{Code: code, Name: "Part " + code, Status: status, Version: 1},
		Facility:  "Hangar 1", Owner: "Airworthiness team", Category: "engine",
		RiskLevel: "high",
	}
	if err := db.Create(part).Error; err != nil {
		t.Fatalf("create part %s: %v", code, err)
	}
	return part
}

func TestRegisterAssemblyEnforcesForestAndBlocksCycles(t *testing.T) {
	db := newAssemblyTestDB(t)
	assemblySvc := NewPartAssemblyService(repository.NewPartAssemblyRepository(db), nil)
	ctx := context.Background()

	parent := createPartForAssembly(t, db, "ASM-P1", "received")
	child := createPartForAssembly(t, db, "ASM-C1", "released")
	other := createPartForAssembly(t, db, "ASM-O1", "released")

	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: parent.ID, ChildPartID: parent.ID}, "operator", "req-self"); !errors.Is(err, ErrAssemblySelfReference) {
		t.Fatalf("self mounting must be rejected, got %v", err)
	}

	link, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: parent.ID, ChildPartID: child.ID}, "operator", "req-register")
	if err != nil {
		t.Fatalf("register assembly: %v", err)
	}
	if link.Parent.ID != parent.ID || link.Child.ID != child.ID {
		t.Fatalf("unexpected link view: %#v", link)
	}

	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: parent.ID, ChildPartID: child.ID}, "operator", "req-dup"); !errors.Is(err, ErrAssemblyDuplicate) {
		t.Fatalf("duplicate relationship must be rejected, got %v", err)
	}
	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: other.ID, ChildPartID: child.ID}, "operator", "req-second-parent"); !errors.Is(err, ErrAssemblyChildMounted) {
		t.Fatalf("child under second parent must be rejected, got %v", err)
	}

	// Trying to mount the parent below its own child would loop back to itself.
	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: child.ID, ChildPartID: parent.ID}, "operator", "req-cycle"); !errors.Is(err, ErrAssemblyCycle) {
		t.Fatalf("cyclic relationship must be rejected, got %v", err)
	}

	// Multi-level cycle: grandchild -> ... -> parent must also be blocked.
	grand := createPartForAssembly(t, db, "ASM-G1", "released")
	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: child.ID, ChildPartID: grand.ID}, "operator", "req-grand"); err != nil {
		t.Fatalf("grandchild mount should succeed, got %v", err)
	}
	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: grand.ID, ChildPartID: parent.ID}, "operator", "req-deep-cycle"); !errors.Is(err, ErrAssemblyCycle) {
		t.Fatalf("deep cyclic relationship must be rejected, got %v", err)
	}

	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: parent.ID, ChildPartID: 999999}, "operator", "req-missing"); !errors.Is(err, ErrAssemblyPartNotFound) {
		t.Fatalf("missing referenced part must be rejected, got %v", err)
	}
}

func TestCheckReleaseReadinessWalksEveryLevel(t *testing.T) {
	db := newAssemblyTestDB(t)
	svc := NewPartAssemblyService(repository.NewPartAssemblyRepository(db), nil)
	ctx := context.Background()

	component := createPartForAssembly(t, db, "CHK-ROOT", "received")
	releasedChild := createPartForAssembly(t, db, "CHK-REL", "released")
	heldChild := createPartForAssembly(t, db, "CHK-HOLD", "hold")
	retiredChild := createPartForAssembly(t, db, "CHK-RET", "retired")
	inspectionGrand := createPartForAssembly(t, db, "CHK-INSPECT", "inspection")
	mount := func(parent, child *model.AircraftPart) {
		if _, err := svc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: parent.ID, ChildPartID: child.ID}, "operator", "req"); err != nil {
			t.Fatalf("mount %s under %s: %v", child.Code, parent.Code, err)
		}
	}
	mount(component, releasedChild)
	mount(component, heldChild)
	mount(component, retiredChild)
	mount(releasedChild, inspectionGrand)

	result, err := svc.CheckReleaseReadiness(ctx, component.ID)
	if err != nil {
		t.Fatalf("readiness check: %v", err)
	}
	if result.Ready || result.Checked != 4 {
		t.Fatalf("expected 4 checked descendants and not ready, got %#v", result)
	}
	byCode := map[string]dto.AssemblyBlockedPart{}
	for _, blocked := range result.Blocked {
		byCode[blocked.Code] = blocked
	}
	held, ok := byCode["CHK-HOLD"]
	if !ok || held.Reason != "暂停" || held.Level != 1 {
		t.Fatalf("hold child missing from blocked list: %#v", result.Blocked)
	}
	retired, ok := byCode["CHK-RET"]
	if !ok || retired.Reason != "退役" || retired.Level != 1 {
		t.Fatalf("retired child missing from blocked list: %#v", result.Blocked)
	}
	grand, ok := byCode["CHK-INSPECT"]
	if !ok || grand.Reason != "未放行" || grand.Level != 2 {
		t.Fatalf("unreleased level-2 grandchild missing from blocked list: %#v", result.Blocked)
	}
	if _, blocked := byCode["CHK-REL"]; blocked {
		t.Fatalf("released child must not block: %#v", result.Blocked)
	}

	view, err := svc.GetPartAssembly(ctx, component.ID)
	if err != nil {
		t.Fatalf("get assembly view: %v", err)
	}
	if len(view.Children) != 3 || view.Parent != nil {
		t.Fatalf("unexpected assembly view: %#v", view)
	}
	if view.Check.Ready != result.Ready || view.Check.Checked != result.Checked || len(view.Check.Blocked) != len(result.Blocked) {
		t.Fatalf("view check result mismatch: %#v vs %#v", view.Check, result)
	}
}

func TestReviewerApprovalBlockedByAssemblyUntilDescendantsReleased(t *testing.T) {
	db := newAssemblyTestDB(t)
	assemblyRepo := repository.NewPartAssemblyRepository(db)
	assemblySvc := NewPartAssemblyService(assemblyRepo, nil)
	authSvc := NewReleaseAuthorizationService(repository.NewReleaseAuthorizationRepository(db), nil, assemblySvc)
	ctx := context.Background()

	component := createPartForAssembly(t, db, "REL-ROOT", "received")
	child := createPartForAssembly(t, db, "REL-CHILD", "hold")
	if _, err := assemblySvc.Register(ctx, dto.RegisterPartAssembly{ParentPartID: component.ID, ChildPartID: child.ID}, "operator", "req-mount"); err != nil {
		t.Fatalf("register assembly: %v", err)
	}

	partID := component.ID
	created, err := authSvc.Create(ctx, dto.CreateReleaseAuthorization{
		Code: "REL-AUTH-1", Name: "Component release", Facility: "Hangar 2", Owner: "Release desk",
		Category: "engine", RiskLevel: "high", EffectiveAt: time.Now().UTC(), Evidence: "evidence",
		AircraftPartID: &partID,
	}, "operator", "req-create")
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}
	review, err := authSvc.Transition(ctx, created.ID, dto.TransitionRequest{Status: "review", ExpectedVersion: 1}, "operator", model.RoleOperator, "req-review")
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}

	_, err = authSvc.Transition(ctx, review.ID, dto.TransitionRequest{Status: "approved", ExpectedVersion: review.Version, Reason: "review"}, "reviewer", model.RoleReviewer, "req-approve-blocked")
	var blocked *AssemblyBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("approval must return AssemblyBlockedError, got %v", err)
	}
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].Code != "REL-CHILD" {
		t.Fatalf("blocked list must identify REL-CHILD, got %#v", blocked.Blocked)
	}

	stuck, err := authSvc.Get(ctx, review.ID)
	if err != nil {
		t.Fatalf("reload authorization: %v", err)
	}
	if stuck.Status != "review" || stuck.Version != review.Version {
		t.Fatalf("authorization must stay in review without a new version, got status=%s version=%d", stuck.Status, stuck.Version)
	}

	if err := db.Model(&model.AircraftPart{}).Where("id = ?", child.ID).Update("status", "released").Error; err != nil {
		t.Fatalf("release child: %v", err)
	}
	approved, err := authSvc.Transition(ctx, review.ID, dto.TransitionRequest{Status: "approved", ExpectedVersion: review.Version, Reason: "review"}, "reviewer", model.RoleReviewer, "req-approve-ok")
	if err != nil {
		t.Fatalf("approval after children released should succeed, got %v", err)
	}
	if approved.Status != "approved" {
		t.Fatalf("expected approved, got %s", approved.Status)
	}
}
