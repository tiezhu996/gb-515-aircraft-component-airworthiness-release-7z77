package model

import "time"

// PartAssembly 登记部件之间的装配关系：ParentPartID 是组件，ChildPartID 是
// 装在该组件下的子件。ChildPartID 上的唯一索引保证同一个子件不会同时挂在
// 两个组件下面；环（包括把自己挂到自己下面）由服务层图遍历拦截。
type PartAssembly struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ParentPartID uint      `json:"parentPartId" gorm:"not null;index;uniqueIndex:idx_part_assembly_pair,priority:1"`
	ChildPartID  uint      `json:"childPartId" gorm:"not null;uniqueIndex;uniqueIndex:idx_part_assembly_pair,priority:2"`
	CreatedBy    string    `json:"createdBy" gorm:"size:80;not null"`
	RequestID    string    `json:"requestId" gorm:"size:64;not null"`
	CreatedAt    time.Time `json:"createdAt" gorm:"index"`
}

func (PartAssembly) TableName() string { return "part_assemblies" }
