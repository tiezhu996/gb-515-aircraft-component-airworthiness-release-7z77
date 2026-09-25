package service

import (
	"context"
	"errors"
	"testing"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newAssemblyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

func seedPart(t *testing.T, db *gorm.DB, code, status string) model.AircraftPart {
	t.Helper()
	part := model.AircraftPart{
		BaseModel: model.BaseModel{Code: code, Name: "部件 " + code, Status: status, Version: 1},
		Facility: "Hangar 1", Owner: "Airworthiness team", Category: "engine", RiskLevel: "high",
	}
	if err := db.Create(&part).Error; err != nil {
		t.Fatalf("seed part %s: %v", code, err)
	}
	return part
}

func newAssemblyFixture(t *testing.T) (PartAssemblyService, *gorm.DB) {
	db := newAssemblyTestDB(t)
	partRepo := repository.NewAircraftPartRepository(db)
	assemblyRepo := repository.NewPartAssemblyRepository(db)
	svc := NewPartAssemblyService(assemblyRepo, partRepo, nil)
	return svc, db
}

func register(t *testing.T, svc PartAssemblyService, parent, child string) {
	t.Helper()
	if _, err := svc.Register(context.Background(), dto.RegisterAssembly{ParentCode: parent, ChildCode: child}, "operator", "assembly-test"); err != nil {
		t.Fatalf("register %s -> %s: %v", parent, child, err)
	}
}

func TestAssemblyCycleAndUniquenessAndVerification(t *testing.T) {
	svc, db := newAssemblyFixture(t)
	ctx := context.Background()

	root := seedPart(t, db, "ASM-ROOT", "released")
	middle := seedPart(t, db, "ASM-MID", "released")
	holdChild := seedPart(t, db, "ASM-HOLD", "hold")
	receivedChild := seedPart(t, db, "ASM-RECV", "received")
	retiredChild := seedPart(t, db, "ASM-RET", "retired")
	other := seedPart(t, db, "ASM-OTHER", "released")

	// 1. 自环必须被挡住。
	if _, err := svc.Register(ctx, dto.RegisterAssembly{ParentCode: root.Code, ChildCode: root.Code}, "operator", "self"); !errors.Is(err, ErrAssemblyCycle) {
		t.Fatalf("self link must be rejected, got %v", err)
	}

	register(t, svc, root.Code, middle.Code)
	register(t, svc, middle.Code, holdChild.Code)
	register(t, svc, middle.Code, receivedChild.Code)

	// 2. 绕回自己（孙 -> 祖父）必须被挡住。
	if _, err := svc.Register(ctx, dto.RegisterAssembly{ParentCode: holdChild.Code, ChildCode: root.Code}, "operator", "cycle"); !errors.Is(err, ErrAssemblyCycle) {
		t.Fatalf("cycle link must be rejected, got %v", err)
	}

	// 3. 同一子件不能挂在两个组件下。
	if _, err := svc.Register(ctx, dto.RegisterAssembly{ParentCode: other.Code, ChildCode: middle.Code}, "operator", "dup-parent"); !errors.Is(err, ErrAssemblyConflict) {
		t.Fatalf("child mounted under two components must be rejected, got %v", err)
	}

	// 4. 重复登记同一关系。
	if _, err := svc.Register(ctx, dto.RegisterAssembly{ParentCode: root.Code, ChildCode: middle.Code}, "operator", "dup-link"); !errors.Is(err, ErrAssemblyDuplicate) {
		t.Fatalf("duplicate link must be rejected, got %v", err)
	}

	// 5. 逐级核对：hold 与 received 后代卡住，列出编号。
	allClear, blocked, err := svc.VerifyRelease(ctx, root.ID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if allClear {
		t.Fatalf("verification must fail with hold/received descendants")
	}
	blockedCodes := map[string]string{}
	for _, item := range blocked {
		blockedCodes[item.Code] = item.Reason
	}
	if blockedCodes["ASM-HOLD"] != "部件已暂停" {
		t.Fatalf("hold part must be reported, got %#v", blockedCodes)
	}
	if blockedCodes["ASM-RECV"] != "部件尚未放行" {
		t.Fatalf("unreleased part must be reported, got %#v", blockedCodes)
	}

	// 6. 退役子件单独挂在另一个组件下也应被识别。
	register(t, svc, other.Code, retiredChild.Code)
	_, blockedOther, err := svc.VerifyRelease(ctx, other.ID)
	if err != nil {
		t.Fatalf("verify other: %v", err)
	}
	foundRetired := false
	for _, item := range blockedOther {
		if item.Code == "ASM-RET" && item.Reason == "部件已退役" {
			foundRetired = true
		}
	}
	if !foundRetired {
		t.Fatalf("retired part must block release, got %#v", blockedOther)
	}

	// 7. 所有后代放行后核对通过。
	holdChild.Status = "released"
	if err := db.Model(&model.AircraftPart{}).Where("id = ?", holdChild.ID).Update("status", "released").Error; err != nil {
		t.Fatalf("release hold part: %v", err)
	}
	if err := db.Model(&model.AircraftPart{}).Where("id = ?", receivedChild.ID).Update("status", "released").Error; err != nil {
		t.Fatalf("release received part: %v", err)
	}
	allClear, blocked, err = svc.VerifyRelease(ctx, root.ID)
	if err != nil {
		t.Fatalf("verify after release: %v", err)
	}
	if !allClear || len(blocked) != 0 {
		t.Fatalf("verification must pass once all descendants are released, blocked=%#v", blocked)
	}
}

func TestAssemblyViewReportsTreeAndParents(t *testing.T) {
	svc, db := newAssemblyFixture(t)
	root := seedPart(t, db, "VIEW-ROOT", "released")
	child := seedPart(t, db, "VIEW-CHILD", "released")
	register(t, svc, root.Code, child.Code)

	view, err := svc.GetView(context.Background(), child.ID)
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if len(view.Parents) != 1 || view.Parents[0].Code != root.Code {
		t.Fatalf("child must report its parent, got %#v", view.Parents)
	}
	if !view.AllClear {
		t.Fatalf("released-only tree must be all clear")
	}

	viewRoot, err := svc.GetView(context.Background(), root.ID)
	if err != nil {
		t.Fatalf("get root view: %v", err)
	}
	if len(viewRoot.Children) != 1 || viewRoot.Children[0].Code != child.Code {
		t.Fatalf("root must list its child, got %#v", viewRoot.Children)
	}
}
