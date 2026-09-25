package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
)

type AircraftPartService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.AircraftPart], error)
	Get(context.Context, uint) (model.AircraftPart, error)
	Create(context.Context, dto.CreateAircraftPart, string, string) (model.AircraftPart, error)
	Update(context.Context, uint, dto.UpdateAircraftPart, string, string) (model.AircraftPart, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.AircraftPart, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	SetAssemblyCleaner(AssemblyCleaner)
}

// AssemblyCleaner 抽象装配关系清理，避免部件服务直接依赖装配仓储细节。
type AssemblyCleaner interface {
	DeleteByPart(context.Context, uint) error
}

type aircraftPartService struct {
	repository repository.AircraftPartRepository
	assemblies AssemblyCleaner
	security   SecurityService
}

func NewAircraftPartService(repo repository.AircraftPartRepository, security SecurityService) AircraftPartService {
	return &aircraftPartService{repository: repo, security: security}
}

// SetAssemblyCleaner 装配删除部件时的关系清理，由路由装配阶段注入。
func (s *aircraftPartService) SetAssemblyCleaner(cleaner AssemblyCleaner) {
	s.assemblies = cleaner
}

func (s *aircraftPartService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.AircraftPart], error) {
	return s.repository.List(ctx, query)
}

func (s *aircraftPartService) Get(ctx context.Context, id uint) (model.AircraftPart, error) {
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) Create(ctx context.Context, input dto.CreateAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	if err := validateAircraftPartBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.AircraftPart{}, err
	}
	item := model.AircraftPart{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.AircraftPartInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.AircraftPart{}, fmt.Errorf("create 航空部件: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "AircraftPart", item.ID, "", item.Status, "created 航空部件")
	return item, nil
}

func (s *aircraftPartService) Update(ctx context.Context, id uint, input dto.UpdateAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	if err := validateAircraftPartBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.AircraftPart{}, err
	}
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
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.AircraftPart{}, fmt.Errorf("update 航空部件: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "AircraftPart", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.AircraftPartTransitions, current.Status, target) {
		return model.AircraftPart{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.AircraftPart{}, fmt.Errorf("transition 航空部件: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "AircraftPart", id, before, target, input.Reason); err != nil {
		return model.AircraftPart{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	// 部件删除后其上下游装配关系必须一并摘除，避免核对落到不存在的部件。
	if s.assemblies != nil {
		if err := s.assemblies.DeleteByPart(ctx, id); err != nil {
			return err
		}
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "AircraftPart", id, current.Status, "deleted", "soft deleted 航空部件")
}

func (s *aircraftPartService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateAircraftPartBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
