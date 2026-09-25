package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"gorm.io/gorm"
)

// AssemblyNode 是装配树的一个节点，部件页据此展示装配清单与逐级核对结果。
type AssemblyNode struct {
	PartID   uint           `json:"partId"`
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Depth    int            `json:"depth"`
	Blocked  *BlockedPart   `json:"blocked,omitempty"`
	Children []AssemblyNode `json:"children"`
}

// AssemblyView 汇总一个部件向下的装配清单、向上的挂载位置和核对结论。
type AssemblyView struct {
	PartID   uint           `json:"partId"`
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Parents  []AssemblyNode `json:"parents"`
	Children []AssemblyNode `json:"children"`
	AllClear bool           `json:"allClear"`
	Blocked  []BlockedPart  `json:"blocked"`
}

type PartAssemblyService interface {
	Register(ctx context.Context, input dto.RegisterAssembly, actor, requestID string) (AssemblyView, error)
	Remove(ctx context.Context, parentPartID, childPartID uint, actor, requestID string) error
	GetView(ctx context.Context, partID uint) (AssemblyView, error)
	// VerifyRelease 顺着装配关系逐级核对组件部件的全部后代，返回卡住的部件清单。
	VerifyRelease(ctx context.Context, rootPartID uint) (allClear bool, blocked []BlockedPart, err error)
}

type partAssemblyService struct {
	assemblies repository.PartAssemblyRepository
	parts      repository.AircraftPartRepository
	security   SecurityService
}

func NewPartAssemblyService(assemblies repository.PartAssemblyRepository, parts repository.AircraftPartRepository, security SecurityService) PartAssemblyService {
	return &partAssemblyService{assemblies: assemblies, parts: parts, security: security}
}

func (s *partAssemblyService) Register(ctx context.Context, input dto.RegisterAssembly, actor, requestID string) (AssemblyView, error) {
	parentCode := strings.ToUpper(strings.TrimSpace(input.ParentCode))
	childCode := strings.ToUpper(strings.TrimSpace(input.ChildCode))
	if parentCode == "" || childCode == "" {
		return AssemblyView{}, ErrInvalidInput
	}
	if parentCode == childCode {
		return AssemblyView{}, fmt.Errorf("%w: %s", ErrAssemblyCycle, childCode)
	}
	parent, err := s.parts.GetByCode(ctx, parentCode)
	if err != nil {
		return AssemblyView{}, err
	}
	child, err := s.parts.GetByCode(ctx, childCode)
	if err != nil {
		return AssemblyView{}, err
	}

	// 同一个子件只能挂在唯一一个组件下面。
	if existing, err := s.assemblies.FindByChild(ctx, child.ID); err == nil {
		if existing.ParentPartID == parent.ID {
			return AssemblyView{}, ErrAssemblyDuplicate
		}
		parentPart, lookupErr := s.parts.Get(ctx, existing.ParentPartID)
		if lookupErr != nil {
			return AssemblyView{}, lookupErr
		}
		return AssemblyView{}, fmt.Errorf("%w: %s 已挂载在 %s 下", ErrAssemblyConflict, child.Code, parentPart.Code)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return AssemblyView{}, err
	}

	// 环检测：从子件向下走，如果能到达父件，登记后就会绕回父件自己。
	if reaches, err := s.reaches(ctx, child.ID, parent.ID, map[uint]bool{}); err != nil {
		return AssemblyView{}, err
	} else if reaches {
		return AssemblyView{}, fmt.Errorf("%w: %s 是 %s 的上级组件", ErrAssemblyCycle, child.Code, parent.Code)
	}

	link := model.PartAssembly{
		ParentPartID: parent.ID, ChildPartID: child.ID,
		CreatedBy: actor, RequestID: requestID,
	}
	if err := s.assemblies.Create(ctx, &link); err != nil {
		if isDuplicateKey(err) {
			return AssemblyView{}, ErrAssemblyConflict
		}
		return AssemblyView{}, fmt.Errorf("register assembly: %w", err)
	}
	if err := s.audit(ctx, actor, requestID, "register-assembly", link.ID,
		parent.Code, child.Code, fmt.Sprintf("mount %s under %s", child.Code, parent.Code)); err != nil {
		return AssemblyView{}, err
	}
	return s.GetView(ctx, parent.ID)
}

func (s *partAssemblyService) audit(ctx context.Context, actor, requestID, action string, entityID uint, before, after, detail string) error {
	if s.security == nil {
		return nil
	}
	return s.security.Audit(ctx, actor, requestID, action, "PartAssembly", entityID, before, after, detail)
}

func (s *partAssemblyService) Remove(ctx context.Context, parentPartID, childPartID uint, actor, requestID string) error {
	parent, err := s.parts.Get(ctx, parentPartID)
	if err != nil {
		return err
	}
	child, err := s.parts.Get(ctx, childPartID)
	if err != nil {
		return err
	}
	if err := s.assemblies.Delete(ctx, parentPartID, childPartID); err != nil {
		return err
	}
	return s.audit(ctx, actor, requestID, "remove-assembly", childPartID,
		parent.Code, child.Code, fmt.Sprintf("unmount %s from %s", child.Code, parent.Code))
}

func (s *partAssemblyService) GetView(ctx context.Context, partID uint) (AssemblyView, error) {
	root, err := s.parts.Get(ctx, partID)
	if err != nil {
		return AssemblyView{}, err
	}
	children, blocked, err := s.buildTree(ctx, root.ID, 1, map[uint]bool{})
	if err != nil {
		return AssemblyView{}, err
	}
	parentLinks, err := s.assemblies.ListParents(ctx, root.ID)
	if err != nil {
		return AssemblyView{}, err
	}
	parents := make([]AssemblyNode, 0, len(parentLinks))
	for _, link := range parentLinks {
		parentPart, err := s.parts.Get(ctx, link.ParentPartID)
		if err != nil {
			return AssemblyView{}, err
		}
		parents = append(parents, AssemblyNode{
			PartID: parentPart.ID, Code: parentPart.Code, Name: parentPart.Name, Status: parentPart.Status, Depth: 0,
		})
	}
	return AssemblyView{
		PartID: root.ID, Code: root.Code, Name: root.Name, Status: root.Status,
		Parents: parents, Children: children, AllClear: len(blocked) == 0, Blocked: blocked,
	}, nil
}

func (s *partAssemblyService) VerifyRelease(ctx context.Context, rootPartID uint) (bool, []BlockedPart, error) {
	_, blocked, err := s.buildTree(ctx, rootPartID, 1, map[uint]bool{})
	if err != nil {
		return false, nil, err
	}
	return len(blocked) == 0, blocked, nil
}

// buildTree 以深度优先方式逐级展开装配关系，并同时核对每个后代的放行状态。
func (s *partAssemblyService) buildTree(ctx context.Context, partID uint, depth int, trail map[uint]bool) ([]AssemblyNode, []BlockedPart, error) {
	links, err := s.assemblies.ListChildren(ctx, partID)
	if err != nil {
		return nil, nil, err
	}
	nodes := make([]AssemblyNode, 0, len(links))
	var blocked []BlockedPart
	for _, link := range links {
		if trail[link.ChildPartID] {
			// 数据里出现环属于登记约束被绕过；跳过，避免死循环。
			continue
		}
		part, err := s.parts.Get(ctx, link.ChildPartID)
		if err != nil {
			return nil, nil, err
		}
		node := AssemblyNode{PartID: part.ID, Code: part.Code, Name: part.Name, Status: part.Status, Depth: depth}
		if reason := releaseBlockReason(part.Status); reason != "" {
			item := BlockedPart{PartID: part.ID, Code: part.Code, Name: part.Name, Status: part.Status, Reason: reason, Depth: depth}
			node.Blocked = &item
			blocked = append(blocked, item)
		}
		nextTrail := cloneTrail(trail)
		nextTrail[partID] = true
		children, childBlocked, err := s.buildTree(ctx, part.ID, depth+1, nextTrail)
		if err != nil {
			return nil, nil, err
		}
		node.Children = children
		blocked = append(blocked, childBlocked...)
		nodes = append(nodes, node)
	}
	return nodes, blocked, nil
}

// reaches 判断从 fromPartID 出发沿装配关系能否到达 targetPartID。
func (s *partAssemblyService) reaches(ctx context.Context, fromPartID, targetPartID uint, seen map[uint]bool) (bool, error) {
	if fromPartID == targetPartID {
		return true, nil
	}
	if seen[fromPartID] {
		return false, nil
	}
	seen[fromPartID] = true
	links, err := s.assemblies.ListChildren(ctx, fromPartID)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		got, err := s.reaches(ctx, link.ChildPartID, targetPartID, seen)
		if err != nil {
			return false, err
		}
		if got {
			return true, nil
		}
	}
	return false, nil
}

func cloneTrail(trail map[uint]bool) map[uint]bool {
	next := make(map[uint]bool, len(trail)+1)
	for key, value := range trail {
		next[key] = value
	}
	return next
}

// releaseBlockReason 返回阻止放行的原因；只有 released 的部件才算放行完成。
func releaseBlockReason(status string) string {
	switch status {
	case "released":
		return ""
	case "hold":
		return "部件已暂停"
	case "retired":
		return "部件已退役"
	default:
		return "部件尚未放行"
	}
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
