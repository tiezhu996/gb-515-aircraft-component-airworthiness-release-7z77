package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"gorm.io/gorm"
)

type ReleaseAuthorizationService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ReleaseAuthorization], error)
	Get(context.Context, uint) (model.ReleaseAuthorization, error)
	Create(context.Context, dto.CreateReleaseAuthorization, string, string) (model.ReleaseAuthorization, error)
	Update(context.Context, uint, dto.UpdateReleaseAuthorization, string, string) (model.ReleaseAuthorization, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.ReleaseAuthorization, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	SetAssemblyReleaseVerifier(AssemblyReleaseVerifier)
	SetPartChecker(PartExistenceChecker)
}

// AssemblyReleaseVerifier 在复核员批准组件放行前顺着装配关系逐级核对。
type AssemblyReleaseVerifier interface {
	VerifyRelease(ctx context.Context, rootPartID uint) (allClear bool, blocked []BlockedPart, err error)
}

// PartExistenceChecker 校验放行授权关联的部件确实存在。
type PartExistenceChecker interface {
	Get(context.Context, uint) (model.AircraftPart, error)
}

type releaseAuthorizationService struct {
	repository repository.ReleaseAuthorizationRepository
	security   SecurityService
	verifier   AssemblyReleaseVerifier
	parts      PartExistenceChecker
}

func NewReleaseAuthorizationService(repo repository.ReleaseAuthorizationRepository, security SecurityService) ReleaseAuthorizationService {
	return &releaseAuthorizationService{repository: repo, security: security}
}

// SetAssemblyReleaseVerifier 注入装配核对依赖，由路由装配阶段调用。
func (s *releaseAuthorizationService) SetAssemblyReleaseVerifier(verifier AssemblyReleaseVerifier) {
	s.verifier = verifier
}

// SetPartChecker 注入部件查询依赖，由路由装配阶段调用。
func (s *releaseAuthorizationService) SetPartChecker(parts PartExistenceChecker) {
	s.parts = parts
}

func (s *releaseAuthorizationService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ReleaseAuthorization], error) {
	return s.repository.List(ctx, query)
}

func (s *releaseAuthorizationService) Get(ctx context.Context, id uint) (model.ReleaseAuthorization, error) {
	return s.repository.Get(ctx, id)
}

func (s *releaseAuthorizationService) Create(ctx context.Context, input dto.CreateReleaseAuthorization, actor, requestID string) (model.ReleaseAuthorization, error) {
	if err := validateReleaseAuthorizationBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ReleaseAuthorization{}, err
	}
	partID, err := s.resolvePartID(ctx, input.PartID)
	if err != nil {
		return model.ReleaseAuthorization{}, err
	}
	item := model.ReleaseAuthorization{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ReleaseAuthorizationInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		PartID: partID,
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.CreateVersion(ctx, &item, actor, requestID); err != nil {
		return model.ReleaseAuthorization{}, fmt.Errorf("create 放行授权: %w", err)
	}
	return s.repository.Get(ctx, item.ID)
}

// resolvePartID 校验授权显式关联的部件存在；未关联时返回 nil。
func (s *releaseAuthorizationService) resolvePartID(ctx context.Context, partID *uint) (*uint, error) {
	if partID == nil || *partID == 0 {
		return nil, nil
	}
	if s.parts == nil {
		return partID, nil
	}
	if _, err := s.parts.Get(ctx, *partID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: partId 对应的部件不存在", ErrInvalidInput)
		}
		return nil, err
	}
	return partID, nil
}

func (s *releaseAuthorizationService) Update(ctx context.Context, id uint, input dto.UpdateReleaseAuthorization, actor, requestID string) (model.ReleaseAuthorization, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ReleaseAuthorization{}, err
	}
	if current.Status != model.ReleaseAuthorizationInitialStatus {
		return model.ReleaseAuthorization{}, ErrLocked
	}
	if err := validateReleaseAuthorizationBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ReleaseAuthorization{}, err
	}
	partID, err := s.resolvePartID(ctx, input.PartID)
	if err != nil {
		return model.ReleaseAuthorization{}, err
	}
	current.PartID = partID
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateVersion(ctx, id, input.ExpectedVersion, &current, actor, requestID, "update", current.Status, "draft authorization fields updated"); err != nil {
		return model.ReleaseAuthorization{}, fmt.Errorf("update 放行授权: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *releaseAuthorizationService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.ReleaseAuthorization, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ReleaseAuthorization{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.ReleaseAuthorizationTransitions, current.Status, target) {
		return model.ReleaseAuthorization{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	if !isOperatorRole(role) {
		return model.ReleaseAuthorization{}, ErrForbidden
	}
	if target == "review" {
		current.SubmittedBy = actor
		current.ReviewedBy = ""
		current.ReviewReason = ""
	}
	if target == "approved" || target == "restricted" || target == "revoked" || target == "draft" {
		if !isReviewerRole(role) {
			return model.ReleaseAuthorization{}, ErrForbidden
		}
		if current.SubmittedBy != "" && actor == current.SubmittedBy {
			return model.ReleaseAuthorization{}, ErrSeparationOfDuty
		}
		// 批准或限制放行组件前，顺着装配关系逐级核对：下层只要有暂停、退役或
		// 尚未放行的部件，授权就留在待复核（review），并把卡住的编号带回去。
		if (target == "approved" || target == "restricted") && s.verifier != nil && current.PartID != nil {
			allClear, blocked, verifyErr := s.verifier.VerifyRelease(ctx, *current.PartID)
			if verifyErr != nil {
				return model.ReleaseAuthorization{}, verifyErr
			}
			if !allClear {
				return model.ReleaseAuthorization{}, &AssemblyBlockedError{Blocked: blocked}
			}
		}
		current.ReviewedBy = actor
		current.ReviewReason = strings.TrimSpace(input.Reason)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateVersion(ctx, id, input.ExpectedVersion, &current, actor, requestID, "transition", before, strings.TrimSpace(input.Reason)); err != nil {
		return model.ReleaseAuthorization{}, fmt.Errorf("transition 放行授权: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *releaseAuthorizationService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Status != model.ReleaseAuthorizationInitialStatus {
		return ErrLocked
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ReleaseAuthorization", id, current.Status, "deleted", "soft deleted 放行授权")
}

func (s *releaseAuthorizationService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateReleaseAuthorizationBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}

func isOperatorRole(role string) bool {
	return role == model.RoleOperator || role == model.RoleReviewer || role == model.RoleAdmin
}

func isReviewerRole(role string) bool {
	return role == model.RoleReviewer || role == model.RoleAdmin
}
