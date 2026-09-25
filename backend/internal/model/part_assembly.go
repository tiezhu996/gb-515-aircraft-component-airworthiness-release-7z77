package model

import "time"

// PartAssembly records the 装配关系 between two 航空部件 records: ParentPart is
// the 组件 and ChildPart is a 子件 mounted below it. A child part can only be
// mounted under a single parent (unique index on child_part_id), which keeps the
// whole graph a forest and makes cycle detection a simple ancestor walk.
type PartAssembly struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ParentPartID uint      `json:"parentPartId" gorm:"not null;uniqueIndex:idx_part_assembly_pair,priority:1;index"`
	ChildPartID  uint      `json:"childPartId" gorm:"not null;uniqueIndex:idx_part_assembly_pair,priority:2;uniqueIndex"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (PartAssembly) TableName() string { return "part_assemblies" }
