package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrForbidden         = errors.New("role is not permitted for this operation")
	ErrLocked            = errors.New("record is locked after review begins")
	ErrSeparationOfDuty  = errors.New("preparer and reviewer must be different users")
	ErrAssemblyCycle     = errors.New("assembly link would loop back to the same component")
	ErrAssemblyConflict  = errors.New("child part is already mounted under another component")
	ErrAssemblyDuplicate = errors.New("assembly link already registered")
)

// BlockedPart describes one descendant part that prevents a component release.
type BlockedPart struct {
	PartID uint   `json:"partId"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Depth  int    `json:"depth"`
}

// AssemblyBlockedError carries the complete list of卡住的部件编号 so handlers
// can keep the authorization in 待复核 and report every blocking part at once.
type AssemblyBlockedError struct {
	Blocked []BlockedPart
}

func (e *AssemblyBlockedError) Error() string {
	return "component release blocked by subparts that are suspended, retired or not released"
}
