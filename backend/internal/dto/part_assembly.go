package dto

// RegisterPartAssembly is the write contract for registering a 组件 -> 子件
// 装配关系. Both endpoints must reference existing parts and are validated
// against self-reference, duplicate mounting and ancestry cycles in the service.
type RegisterPartAssembly struct {
	ParentPartID uint `json:"parentPartId" binding:"required"`
	ChildPartID  uint `json:"childPartId" binding:"required"`
}

// AssemblyPartNode is the compact part projection used in 装配清单 payloads.
type AssemblyPartNode struct {
	ID     uint   `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// AssemblyLinkView pairs a registered 装配关系 with its parent/child part
// snapshots so callers never need a second request to render the edge.
type AssemblyLinkView struct {
	ID        uint             `json:"id"`
	Parent    AssemblyPartNode `json:"parent"`
	Child     AssemblyPartNode `json:"child"`
	CreatedAt string           `json:"createdAt"`
}

// AssemblyBlockedPart explains why a single descendant 子件 blocks release:
// 暂停/退役 states and 未放行 (every state other than released) are reported.
type AssemblyBlockedPart struct {
	ID     uint   `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Level  int    `json:"level"`
}

// PartAssemblyView is the full 部件页 payload: the direct children list, the
// upstream parent (a child has at most one parent) and the level-by-level
// release check result over every descendant.
type PartAssemblyView struct {
	Part     AssemblyPartNode    `json:"part"`
	Parent   *AssemblyPartNode   `json:"parent"`
	Children []AssemblyPartNode  `json:"children"`
	Links    []AssemblyLinkView  `json:"links"`
	Check    AssemblyCheckResult `json:"check"`
}

type AssemblyCheckResult struct {
	Ready   bool                  `json:"ready"`
	Blocked []AssemblyBlockedPart `json:"blocked"`
	Checked int                   `json:"checked"`
}
