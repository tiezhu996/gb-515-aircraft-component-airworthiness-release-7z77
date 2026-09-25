package repository

import (
	"context"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"gorm.io/gorm"
)

// PartAssemblyRepository owns persistence for 部件装配关系.
type PartAssemblyRepository interface {
	Create(ctx context.Context, link *model.PartAssembly) error
	Delete(ctx context.Context, parentPartID, childPartID uint) error
	DeleteByPart(ctx context.Context, partID uint) error
	ListChildren(ctx context.Context, parentPartID uint) ([]model.PartAssembly, error)
	ListParents(ctx context.Context, childPartID uint) ([]model.PartAssembly, error)
	// DirectChildren 返回每个父件 ID 到其直接子件关系的映射。
	DirectChildren(ctx context.Context, parentPartIDs []uint) (map[uint][]model.PartAssembly, error)
	// FindByChild 返回某子件当前的唯一装配关系；不存在时返回 gorm.ErrRecordNotFound。
	FindByChild(ctx context.Context, childPartID uint) (model.PartAssembly, error)
	All(ctx context.Context) ([]model.PartAssembly, error)
}

type partAssemblyRepository struct {
	db *gorm.DB
}

func NewPartAssemblyRepository(db *gorm.DB) PartAssemblyRepository {
	return &partAssemblyRepository{db: db}
}

func (r *partAssemblyRepository) Create(ctx context.Context, link *model.PartAssembly) error {
	return r.db.WithContext(ctx).Create(link).Error
}

func (r *partAssemblyRepository) Delete(ctx context.Context, parentPartID, childPartID uint) error {
	result := r.db.WithContext(ctx).
		Where("parent_part_id = ? AND child_part_id = ?", parentPartID, childPartID).
		Delete(&model.PartAssembly{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *partAssemblyRepository) DeleteByPart(ctx context.Context, partID uint) error {
	return r.db.WithContext(ctx).
		Where("parent_part_id = ? OR child_part_id = ?", partID, partID).
		Delete(&model.PartAssembly{}).Error
}

func (r *partAssemblyRepository) ListChildren(ctx context.Context, parentPartID uint) ([]model.PartAssembly, error) {
	var links []model.PartAssembly
	err := r.db.WithContext(ctx).
		Where("parent_part_id = ?", parentPartID).
		Order("id").Find(&links).Error
	return links, err
}

func (r *partAssemblyRepository) ListParents(ctx context.Context, childPartID uint) ([]model.PartAssembly, error) {
	var links []model.PartAssembly
	err := r.db.WithContext(ctx).
		Where("child_part_id = ?", childPartID).
		Order("id").Find(&links).Error
	return links, err
}

func (r *partAssemblyRepository) DirectChildren(ctx context.Context, parentPartIDs []uint) (map[uint][]model.PartAssembly, error) {
	result := make(map[uint][]model.PartAssembly)
	if len(parentPartIDs) == 0 {
		return result, nil
	}
	var links []model.PartAssembly
	if err := r.db.WithContext(ctx).
		Where("parent_part_id IN ?", parentPartIDs).
		Order("id").Find(&links).Error; err != nil {
		return nil, err
	}
	for _, link := range links {
		result[link.ParentPartID] = append(result[link.ParentPartID], link)
	}
	return result, nil
}

func (r *partAssemblyRepository) FindByChild(ctx context.Context, childPartID uint) (model.PartAssembly, error) {
	var link model.PartAssembly
	err := r.db.WithContext(ctx).Where("child_part_id = ?", childPartID).First(&link).Error
	return link, err
}

func (r *partAssemblyRepository) All(ctx context.Context) ([]model.PartAssembly, error) {
	var links []model.PartAssembly
	err := r.db.WithContext(ctx).Order("id").Find(&links).Error
	return links, err
}
