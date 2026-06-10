// Package actions wraps every relevant app.Service operation as a
// uniform (T, errs.DomainError) action with at most one parameter.
// The factory is constructed once in main and threaded through the
// TUI; screens hold *Actions, never *app.Service.
package actions

import (
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
)

// Actions is the thin forwarding layer between TUI screens and the
// application service. It enforces the single-parameter rule by
// accepting input structs for multi-input operations.
type Actions struct {
	svc *app.Service
}

// New constructs an Actions factory around svc. svc must be non-nil;
// passing nil indicates a wiring bug and is treated as programmer error.
func New(svc *app.Service) *Actions {
	if svc == nil {
		panic("actions.New: nil service")
	}
	return &Actions{svc: svc}
}

// collapse turns an accumulator-style ([]errs.DomainError) return into
// the uniform single-error shape. An empty slice maps to a nil
// interface so callers' `if err != nil` checks still work — returning
// a typed nil errs.Errors would yield a non-nil interface holding a
// nil slice.
func collapse[T any](v T, es []errs.DomainError) (T, errs.DomainError) {
	if len(es) == 0 {
		return v, nil
	}
	return v, errs.Errors(es)
}
