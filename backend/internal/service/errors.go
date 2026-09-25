package service

import (
	"errors"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
)

var (
	ErrInvalidTransition      = errors.New("requested status transition is not allowed")
	ErrInvalidInput           = errors.New("business input validation failed")
	ErrUnauthorized           = errors.New("invalid username or password")
	ErrInactiveUser           = errors.New("user account is inactive")
	ErrForbidden              = errors.New("role is not permitted for this operation")
	ErrLocked                 = errors.New("record is locked after review begins")
	ErrSeparationOfDuty       = errors.New("preparer and reviewer must be different users")
	ErrAssemblySelfReference  = errors.New("a part cannot be assembled under itself")
	ErrAssemblyDuplicate      = errors.New("the child part is already mounted under this parent")
	ErrAssemblyChildMounted   = errors.New("the child part is already mounted under another component")
	ErrAssemblyCycle          = errors.New("the relationship would loop back through the component hierarchy")
	ErrAssemblyPartNotFound   = errors.New("referenced part was not found")
	ErrAssemblyLinkNotFound   = errors.New("assembly relationship was not found")
	ErrAssemblyReleaseBlocked = errors.New("release blocked by suspended, retired or unreleased child parts")
)

// AssemblyBlockedError carries the 装配核对 result so handlers and callers can
// list the exact 子件编号 that keep the authorization in 待复核.
type AssemblyBlockedError struct {
	Err     error
	Blocked []dto.AssemblyBlockedPart
}

func (e *AssemblyBlockedError) Error() string {
	if e == nil || e.Err == nil {
		return ErrAssemblyReleaseBlocked.Error()
	}
	return e.Err.Error()
}

func (e *AssemblyBlockedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
