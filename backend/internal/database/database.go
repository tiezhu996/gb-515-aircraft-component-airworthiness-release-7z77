package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/config"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, cfg config.Config, log *slog.Logger) (*gorm.DB, *redis.Client, error) {
	var dialector gorm.Dialector
	switch cfg.DatabaseDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseDSN)
	case "mysql":
		dialector = mysql.Open(cfg.DatabaseDSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabaseDSN)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
	logLevel := logger.Warn
	if cfg.Environment == "development" {
		logLevel = logger.Info
	}
	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 20; attempt++ {
		db, err = gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil && sqlDB.PingContext(ctx) == nil {
				break
			}
			if dbErr != nil {
				err = dbErr
			} else {
				err = sqlDB.PingContext(ctx)
			}
		}
		log.Warn("database not ready", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, nil, err
	}
	if err := Seed(ctx, db); err != nil {
		return nil, nil, err
	}
	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return nil, nil, fmt.Errorf("connect redis: %w", err)
		}
	}
	return db, redisClient, nil
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{}, &model.AuditLog{},
		&model.AircraftPart{},
		&model.InspectionTask{},
		&model.CertificateRecord{},
		&model.CertificateRecordRevision{},
		&model.ReleaseAuthorization{},
		&model.ReleaseAuthorizationRevision{},
		&model.PartAssembly{},
	)
}

func Seed(ctx context.Context, db *gorm.DB) error {
	var users int64
	if err := db.WithContext(ctx).Model(&model.User{}).Count(&users).Error; err != nil {
		return err
	}
	if users == 0 {
		password, err := bcrypt.GenerateFromPassword([]byte("Admin123!"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		seedUsers := []model.User{
			{Username: "admin", DisplayName: "系统管理员", PasswordHash: string(password), Role: model.RoleAdmin, Active: true},
			{Username: "reviewer", DisplayName: "质量复核员", PasswordHash: string(password), Role: model.RoleReviewer, Active: true},
			{Username: "operator", DisplayName: "现场操作员", PasswordHash: string(password), Role: model.RoleOperator, Active: true},
			{Username: "viewer", DisplayName: "只读审计员", PasswordHash: string(password), Role: model.RoleViewer, Active: true},
		}
		if err := db.WithContext(ctx).Create(&seedUsers).Error; err != nil {
			return err
		}
	}

	if err := seedAircraftPart(ctx, db); err != nil {
		return err
	}

	if err := seedPartAssembly(ctx, db); err != nil {
		return err
	}

	if err := seedInspectionTask(ctx, db); err != nil {
		return err
	}

	if err := seedCertificateRecord(ctx, db); err != nil {
		return err
	}

	if err := seedReleaseAuthorization(ctx, db); err != nil {
		return err
	}

	if err := seedAuthorizationAssemblyLink(ctx, db); err != nil {
		return err
	}

	return nil
}

func seedAircraftPart(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.AircraftPart{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.AircraftPart{

		{BaseModel: model.BaseModel{Code: "AP-001", Name: "航空部件示例一", Status: "received", Version: 1,
			Description: "用于启动验证和主要流程演示的航空部件记录"}, Facility: "航空部件适航放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-01"},

		{BaseModel: model.BaseModel{Code: "AP-002", Name: "航空部件示例二", Status: "inspection", Version: 1,
			Description: "用于启动验证和主要流程演示的航空部件记录"}, Facility: "航空部件适航放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-02"},

		{BaseModel: model.BaseModel{Code: "AP-003", Name: "航空部件示例三", Status: "hold", Version: 1,
			Description: "用于启动验证和主要流程演示的航空部件记录"}, Facility: "航空部件适航放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-03"},

		// 装配演示：AP-004 是组件，挂 AP-005/AP-006/AP-007 三个子件，
		// 其中 AP-006 暂停、AP-007 退役，用于演示组件放行被卡住。
		{BaseModel: model.BaseModel{Code: "AP-004", Name: "涡扇组件（装配演示）", Status: "received", Version: 1,
			Description: "登记了三个子件的组件，放行复核会逐级核对子件状态"}, Facility: "总装车间", Owner: "装配组",
			Category: "组件", RiskLevel: "high", MetricValue: 100, MetricUnit: "%",
			EffectiveAt: now, Evidence: "组件装配记录", RelatedCode: ""},

		{BaseModel: model.BaseModel{Code: "AP-005", Name: "风扇叶片（已放行子件）", Status: "released", Version: 1,
			Description: "组件 AP-004 的已放行子件"}, Facility: "部件车间", Owner: "运行一组",
			Category: "子件", RiskLevel: "medium", MetricValue: 100, MetricUnit: "%",
			EffectiveAt: now, Evidence: "子件检验通过并已放行", RelatedCode: ""},

		{BaseModel: model.BaseModel{Code: "AP-006", Name: "燃油泵（暂停子件）", Status: "hold", Version: 1,
			Description: "组件 AP-004 的暂停子件，复核时应阻断放行"}, Facility: "部件车间", Owner: "质量复核组",
			Category: "子件", RiskLevel: "high", MetricValue: 0, MetricUnit: "%",
			EffectiveAt: now, Evidence: "故障待查，暂停使用", RelatedCode: ""},

		{BaseModel: model.BaseModel{Code: "AP-007", Name: "作动筒（退役子件）", Status: "retired", Version: 1,
			Description: "组件 AP-004 的退役子件，复核时应阻断放行"}, Facility: "部件车间", Owner: "安全主管组",
			Category: "子件", RiskLevel: "critical", MetricValue: 0, MetricUnit: "%",
			EffectiveAt: now, Evidence: "到寿退役", RelatedCode: ""},
	}
	if err := db.WithContext(ctx).Create(&items).Error; err != nil {
		return err
	}

	// 追加一层孙件：AP-008 挂在已放行的 AP-005 下但尚未放行，演示“逐级核对”
	// 必须穿透多层，而不是只看直接子件。
	grandchild := model.AircraftPart{
		BaseModel: model.BaseModel{Code: "AP-008", Name: "密封环（未放行孙件）", Status: "inspection", Version: 1,
			Description: "AP-005 的子件、AP-004 的孙件，仍在检查中"},
		Facility: "部件车间", Owner: "运行一组", Category: "子件", RiskLevel: "medium",
		MetricValue: 60, MetricUnit: "%", EffectiveAt: now, Evidence: "检查进行中", RelatedCode: "",
	}
	return db.WithContext(ctx).Create(&grandchild).Error
}

func seedPartAssembly(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.PartAssembly{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	idByCode := make(map[string]uint)
	var parts []model.AircraftPart
	if err := db.WithContext(ctx).Where("code IN ?", []string{"AP-004", "AP-005", "AP-006", "AP-007", "AP-008"}).Find(&parts).Error; err != nil {
		return err
	}
	for _, part := range parts {
		idByCode[part.Code] = part.ID
	}
	now := time.Now().UTC()
	links := []model.PartAssembly{
		{ParentPartID: idByCode["AP-004"], ChildPartID: idByCode["AP-005"], CreatedAt: now},
		{ParentPartID: idByCode["AP-004"], ChildPartID: idByCode["AP-006"], CreatedAt: now},
		{ParentPartID: idByCode["AP-004"], ChildPartID: idByCode["AP-007"], CreatedAt: now},
		{ParentPartID: idByCode["AP-005"], ChildPartID: idByCode["AP-008"], CreatedAt: now},
	}
	if err := db.WithContext(ctx).Create(&links).Error; err != nil {
		return err
	}
	return nil
}

// seedAuthorizationAssemblyLink binds the demo 待复核 authorization to the
// component with blocking children; it must run after 放行授权 seeding.
func seedAuthorizationAssemblyLink(ctx context.Context, db *gorm.DB) error {
	var part model.AircraftPart
	if err := db.WithContext(ctx).Where("code = ?", "AP-004").First(&part).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&model.ReleaseAuthorization{}).
		Where("code = ? AND aircraft_part_id IS NULL", "RA-002").
		Update("aircraft_part_id", part.ID).Error
}

func seedInspectionTask(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.InspectionTask{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.InspectionTask{

		{BaseModel: model.BaseModel{Code: "IT-001", Name: "检查任务示例一", Status: "planned", Version: 1,
			Description: "用于启动验证和主要流程演示的检查任务记录"}, Facility: "航空部件适航放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-01"},

		{BaseModel: model.BaseModel{Code: "IT-002", Name: "检查任务示例二", Status: "running", Version: 1,
			Description: "用于启动验证和主要流程演示的检查任务记录"}, Facility: "航空部件适航放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-02"},

		{BaseModel: model.BaseModel{Code: "IT-003", Name: "检查任务示例三", Status: "passed", Version: 1,
			Description: "用于启动验证和主要流程演示的检查任务记录"}, Facility: "航空部件适航放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-03"},
	}
	return db.WithContext(ctx).Create(&items).Error
}

func seedCertificateRecord(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.CertificateRecord{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.CertificateRecord{

		{BaseModel: model.BaseModel{Code: "CR-001", Name: "证书记录示例一", Status: "draft", Version: 1,
			Description: "用于启动验证和主要流程演示的证书记录记录"}, Facility: "航空部件适航放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-01", PreparedBy: "operator"},

		{BaseModel: model.BaseModel{Code: "CR-002", Name: "证书记录示例二", Status: "valid", Version: 1,
			Description: "用于启动验证和主要流程演示的证书记录记录"}, Facility: "航空部件适航放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-02", PreparedBy: "operator", VerifiedBy: "reviewer"},

		{BaseModel: model.BaseModel{Code: "CR-003", Name: "证书记录示例三", Status: "expired", Version: 1,
			Description: "用于启动验证和主要流程演示的证书记录记录"}, Facility: "航空部件适航放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-03", PreparedBy: "operator", VerifiedBy: "reviewer"},
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		revisions := make([]model.CertificateRecordRevision, 0, len(items))
		for _, item := range items {
			revisions = append(revisions, model.CertificateRecordRevision{
				CertificateRecordID: item.ID, Version: item.Version, Status: item.Status,
				Evidence: item.Evidence, Actor: "system-seed", RequestID: "seed-gb-515",
				Action: "seed", Reason: "initial demonstration certificate", CreatedAt: now,
			})
		}
		return tx.Create(&revisions).Error
	})
}

func seedReleaseAuthorization(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.ReleaseAuthorization{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.ReleaseAuthorization{

		{BaseModel: model.BaseModel{Code: "RA-001", Name: "放行授权示例一", Status: "draft", Version: 1,
			Description: "用于启动验证和主要流程演示的放行授权记录"}, Facility: "航空部件适航放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-01"},

		{BaseModel: model.BaseModel{Code: "RA-002", Name: "放行授权示例二", Status: "review", Version: 1,
			Description: "用于启动验证和主要流程演示的放行授权记录"}, Facility: "航空部件适航放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-02", SubmittedBy: "operator"},

		{BaseModel: model.BaseModel{Code: "RA-003", Name: "放行授权示例三", Status: "approved", Version: 1,
			Description: "用于启动验证和主要流程演示的放行授权记录"}, Facility: "航空部件适航放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-515-03", SubmittedBy: "operator", ReviewedBy: "reviewer", ReviewReason: "演示数据双人复核通过"},
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		revisions := make([]model.ReleaseAuthorizationRevision, 0, len(items))
		for _, item := range items {
			revisions = append(revisions, model.ReleaseAuthorizationRevision{
				ReleaseAuthorizationID: item.ID, Version: item.Version, Status: item.Status,
				Evidence: item.Evidence, Actor: "system-seed", RequestID: "seed-gb-515",
				Action: "seed", Reason: "initial demonstration authorization", CreatedAt: now,
			})
		}
		return tx.Create(&revisions).Error
	})
}
