package repository

import (
	"context"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"gorm.io/gorm"
)

// PartAssemblyRepository owns persistence for 部件装配关系. The assembly graph
// is a forest (each child has at most one parent), so the repository only needs
// flat edge reads plus the unique constraint enforced by the model indexes.
type PartAssemblyRepository interface {
	ListLinks(context.Context) ([]model.PartAssembly, error)
	FindLink(ctx context.Context, parentPartID, childPartID uint) (model.PartAssembly, error)
	CreateLink(context.Context, *model.PartAssembly) error
	DeleteLink(ctx context.Context, id uint) error
	FindPartsByIDs(ctx context.Context, ids []uint) ([]model.AircraftPart, error)
	GetPart(ctx context.Context, id uint) (model.AircraftPart, error)
}

type partAssemblyRepository struct {
	db *gorm.DB
}

func NewPartAssemblyRepository(db *gorm.DB) PartAssemblyRepository {
	return &partAssemblyRepository{db: db}
}

func (r *partAssemblyRepository) ListLinks(ctx context.Context) ([]model.PartAssembly, error) {
	links := make([]model.PartAssembly, 0)
	err := r.db.WithContext(ctx).Order("id").Find(&links).Error
	return links, err
}

func (r *partAssemblyRepository) FindLink(ctx context.Context, parentPartID, childPartID uint) (model.PartAssembly, error) {
	var link model.PartAssembly
	err := r.db.WithContext(ctx).
		Where("parent_part_id = ? AND child_part_id = ?", parentPartID, childPartID).
		First(&link).Error
	return link, err
}

func (r *partAssemblyRepository) CreateLink(ctx context.Context, link *model.PartAssembly) error {
	return r.db.WithContext(ctx).Create(link).Error
}

func (r *partAssemblyRepository) DeleteLink(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.PartAssembly{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *partAssemblyRepository) FindPartsByIDs(ctx context.Context, ids []uint) ([]model.AircraftPart, error) {
	parts := make([]model.AircraftPart, 0, len(ids))
	if len(ids) == 0 {
		return parts, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&parts).Error
	return parts, err
}

func (r *partAssemblyRepository) GetPart(ctx context.Context, id uint) (model.AircraftPart, error) {
	var part model.AircraftPart
	err := r.db.WithContext(ctx).First(&part, id).Error
	return part, err
}
