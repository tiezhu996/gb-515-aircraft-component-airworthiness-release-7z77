package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"gorm.io/gorm"
)

// ReleaseGate lets the 放行授权 service verify 装配层级 without importing the
// assembly service implementation. Every descendant 子件 must be released
// before a component authorization can leave 待复核.
type ReleaseGate interface {
	PartExists(context.Context, uint) (bool, error)
	CheckReleaseReadiness(context.Context, uint) (dto.AssemblyCheckResult, error)
}

type PartAssemblyService interface {
	ReleaseGate
	Register(context.Context, dto.RegisterPartAssembly, string, string) (dto.AssemblyLinkView, error)
	Remove(ctx context.Context, linkID uint, actor, requestID string) error
	GetPartAssembly(context.Context, uint) (dto.PartAssemblyView, error)
}

type partAssemblyService struct {
	repository repository.PartAssemblyRepository
	security   SecurityService
}

func NewPartAssemblyService(repo repository.PartAssemblyRepository, security SecurityService) PartAssemblyService {
	return &partAssemblyService{repository: repo, security: security}
}

func (s *partAssemblyService) Register(ctx context.Context, input dto.RegisterPartAssembly, actor, requestID string) (dto.AssemblyLinkView, error) {
	if input.ParentPartID == 0 || input.ChildPartID == 0 {
		return dto.AssemblyLinkView{}, ErrInvalidInput
	}
	if input.ParentPartID == input.ChildPartID {
		return dto.AssemblyLinkView{}, ErrAssemblySelfReference
	}
	parts, err := s.repository.FindPartsByIDs(ctx, []uint{input.ParentPartID, input.ChildPartID})
	if err != nil {
		return dto.AssemblyLinkView{}, fmt.Errorf("load assembly parts: %w", err)
	}
	if len(parts) != 2 {
		return dto.AssemblyLinkView{}, ErrAssemblyPartNotFound
	}

	links, err := s.repository.ListLinks(ctx)
	if err != nil {
		return dto.AssemblyLinkView{}, fmt.Errorf("load assembly graph: %w", err)
	}
	parentByChild := make(map[uint]model.PartAssembly, len(links))
	for _, link := range links {
		parentByChild[link.ChildPartID] = link
	}
	if existing, mounted := parentByChild[input.ChildPartID]; mounted {
		if existing.ParentPartID == input.ParentPartID {
			return dto.AssemblyLinkView{}, ErrAssemblyDuplicate
		}
		return dto.AssemblyLinkView{}, ErrAssemblyChildMounted
	}
	// Walk up from the proposed parent: mounting the child above an ancestor
	// would close a loop back through the component itself. seen guards against
	// pathological pre-existing cycles so the walk can never spin forever.
	for ancestorID, seen := input.ParentPartID, map[uint]bool{}; ; {
		if ancestorID == input.ChildPartID {
			return dto.AssemblyLinkView{}, ErrAssemblyCycle
		}
		if seen[ancestorID] {
			return dto.AssemblyLinkView{}, ErrAssemblyCycle
		}
		seen[ancestorID] = true
		edge, ok := parentByChild[ancestorID]
		if !ok {
			break
		}
		ancestorID = edge.ParentPartID
	}

	link := model.PartAssembly{ParentPartID: input.ParentPartID, ChildPartID: input.ChildPartID, CreatedAt: time.Now().UTC()}
	if err := s.repository.CreateLink(ctx, &link); err != nil {
		return dto.AssemblyLinkView{}, fmt.Errorf("register 装配关系: %w", err)
	}
	// Concurrent registrations can both pass the in-memory ancestor check
	// before either commits; re-validate against the committed graph and undo
	// the edge if it closed a loop.
	if s.edgeCreatesCycle(ctx, link) {
		_ = s.repository.DeleteLink(ctx, link.ID)
		return dto.AssemblyLinkView{}, ErrAssemblyCycle
	}
	view := buildLinkView(link, parts)
	if s.security != nil {
		_ = s.security.Audit(ctx, actor, requestID, "register", "PartAssembly", link.ID, "", "registered",
			fmt.Sprintf("mounted child part %d under parent part %d", input.ChildPartID, input.ParentPartID))
	}
	return view, nil
}

func (s *partAssemblyService) Remove(ctx context.Context, linkID uint, actor, requestID string) error {
	if linkID == 0 {
		return ErrInvalidInput
	}
	if err := s.repository.DeleteLink(ctx, linkID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssemblyLinkNotFound
		}
		return err
	}
	if s.security != nil {
		return s.security.Audit(ctx, actor, requestID, "remove", "PartAssembly", linkID, "registered", "removed", "removed 装配关系")
	}
	return nil
}

func (s *partAssemblyService) PartExists(ctx context.Context, id uint) (bool, error) {
	if _, err := s.repository.GetPart(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckReleaseReadiness walks every descendant of partID level by level. A
// descendant blocks release when it is 暂停(hold), 退役(retired) or simply not
// 放行 (any status other than released).
func (s *partAssemblyService) CheckReleaseReadiness(ctx context.Context, partID uint) (dto.AssemblyCheckResult, error) {
	links, err := s.repository.ListLinks(ctx)
	if err != nil {
		return dto.AssemblyCheckResult{}, fmt.Errorf("load assembly graph: %w", err)
	}
	childrenByParent := make(map[uint][]model.PartAssembly)
	ids := map[uint]struct{}{}
	for _, link := range links {
		childrenByParent[link.ParentPartID] = append(childrenByParent[link.ParentPartID], link)
		ids[link.ChildPartID] = struct{}{}
		ids[link.ParentPartID] = struct{}{}
	}
	if _, err := s.repository.GetPart(ctx, partID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.AssemblyCheckResult{}, ErrAssemblyPartNotFound
		}
		return dto.AssemblyCheckResult{}, err
	}

	idList := make([]uint, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	parts, err := s.repository.FindPartsByIDs(ctx, idList)
	if err != nil {
		return dto.AssemblyCheckResult{}, err
	}
	partByID := make(map[uint]model.AircraftPart, len(parts))
	for _, part := range parts {
		partByID[part.ID] = part
	}

	result := dto.AssemblyCheckResult{Ready: true, Blocked: []dto.AssemblyBlockedPart{}}
	type frame struct {
		id    uint
		level int
	}
	queue := make([]frame, 0)
	for _, edge := range childrenByParent[partID] {
		queue = append(queue, frame{id: edge.ChildPartID, level: 1})
	}
	visited := map[uint]bool{partID: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current.id] {
			continue
		}
		visited[current.id] = true
		result.Checked++
		part := partByID[current.id]
		if reason := blockedReason(part.Status); reason != "" {
			result.Ready = false
			result.Blocked = append(result.Blocked, dto.AssemblyBlockedPart{
				ID: part.ID, Code: part.Code, Name: part.Name, Status: part.Status,
				Reason: reason, Level: current.level,
			})
		}
		for _, edge := range childrenByParent[current.id] {
			queue = append(queue, frame{id: edge.ChildPartID, level: current.level + 1})
		}
	}
	sort.SliceStable(result.Blocked, func(i, j int) bool {
		if result.Blocked[i].Level != result.Blocked[j].Level {
			return result.Blocked[i].Level < result.Blocked[j].Level
		}
		return result.Blocked[i].Code < result.Blocked[j].Code
	})
	return result, nil
}

func (s *partAssemblyService) GetPartAssembly(ctx context.Context, partID uint) (dto.PartAssemblyView, error) {
	root, err := s.repository.GetPart(ctx, partID)
	if err != nil {
		return dto.PartAssemblyView{}, err
	}
	links, err := s.repository.ListLinks(ctx)
	if err != nil {
		return dto.PartAssemblyView{}, fmt.Errorf("load assembly graph: %w", err)
	}
	childrenByParent := make(map[uint][]model.PartAssembly)
	parentByChild := make(map[uint]model.PartAssembly)
	idSet := map[uint]struct{}{partID: {}}
	for _, link := range links {
		childrenByParent[link.ParentPartID] = append(childrenByParent[link.ParentPartID], link)
		parentByChild[link.ChildPartID] = link
		idSet[link.ParentPartID] = struct{}{}
		idSet[link.ChildPartID] = struct{}{}
	}

	ids := make([]uint, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	parts, err := s.repository.FindPartsByIDs(ctx, ids)
	if err != nil {
		return dto.PartAssemblyView{}, err
	}
	partByID := make(map[uint]model.AircraftPart, len(parts))
	for _, part := range parts {
		partByID[part.ID] = part
	}

	check, err := s.CheckReleaseReadiness(ctx, partID)
	if err != nil {
		return dto.PartAssemblyView{}, err
	}

	view := dto.PartAssemblyView{
		Part:     toAssemblyNode(root),
		Children: []dto.AssemblyPartNode{},
		Links:    []dto.AssemblyLinkView{},
		Check:    check,
	}
	if edge, ok := parentByChild[partID]; ok {
		if parent, found := partByID[edge.ParentPartID]; found {
			node := toAssemblyNode(parent)
			view.Parent = &node
		}
	}
	childEdges := childrenByParent[partID]
	sort.Slice(childEdges, func(i, j int) bool {
		left, right := partByID[childEdges[i].ChildPartID], partByID[childEdges[j].ChildPartID]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return childEdges[i].ID < childEdges[j].ID
	})
	for _, edge := range childEdges {
		child, found := partByID[edge.ChildPartID]
		if !found {
			continue
		}
		view.Children = append(view.Children, toAssemblyNode(child))
		view.Links = append(view.Links, buildLinkView(edge, []model.AircraftPart{root, child}))
	}
	return view, nil
}

// edgeCreatesCycle reloads the committed graph after insert and checks whether
// the new child can reach its parent through existing ancestors.
func (s *partAssemblyService) edgeCreatesCycle(ctx context.Context, link model.PartAssembly) bool {
	links, err := s.repository.ListLinks(ctx)
	if err != nil {
		return false
	}
	parentByChild := make(map[uint]model.PartAssembly, len(links))
	for _, edge := range links {
		parentByChild[edge.ChildPartID] = edge
	}
	seen := map[uint]bool{}
	for ancestorID := link.ParentPartID; ; {
		if ancestorID == link.ChildPartID || seen[ancestorID] {
			return true
		}
		seen[ancestorID] = true
		edge, ok := parentByChild[ancestorID]
		if !ok {
			return false
		}
		ancestorID = edge.ParentPartID
	}
}

func blockedReason(status string) string {
	switch status {
	case string(constants.PartStateHold):
		return "暂停"
	case string(constants.PartStateRetired):
		return "退役"
	case string(constants.PartStateReleased):
		return ""
	default:
		return "未放行"
	}
}

func toAssemblyNode(part model.AircraftPart) dto.AssemblyPartNode {
	return dto.AssemblyPartNode{ID: part.ID, Code: part.Code, Name: part.Name, Status: part.Status}
}

func buildLinkView(link model.PartAssembly, parts []model.AircraftPart) dto.AssemblyLinkView {
	byID := make(map[uint]model.AircraftPart, len(parts))
	for _, part := range parts {
		byID[part.ID] = part
	}
	return dto.AssemblyLinkView{
		ID:        link.ID,
		Parent:    toAssemblyNode(byID[link.ParentPartID]),
		Child:     toAssemblyNode(byID[link.ChildPartID]),
		CreatedAt: link.CreatedAt.Format(time.RFC3339),
	}
}

// blockedPartCodes renders the blocking 子件编号 list for audit/detail strings.
func blockedPartCodes(blocked []dto.AssemblyBlockedPart) string {
	codes := make([]string, 0, len(blocked))
	for _, item := range blocked {
		codes = append(codes, fmt.Sprintf("%s(%s:%s)", item.Code, item.Status, item.Reason))
	}
	return strings.Join(codes, ", ")
}
