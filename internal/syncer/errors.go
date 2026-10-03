package syncer

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/klppl/kvist/internal/protocol"
)

// Error is a protocol error with its HTTP status.
type Error struct {
	Status int
	Wire   protocol.Error
}

func (e *Error) Error() string { return e.Wire.Error() }

func newError(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Wire: protocol.Error{Code: code, Message: fmt.Sprintf(format, args...)}}
}

func (e *Error) withDetails(v any) *Error {
	if b, err := json.Marshal(v); err == nil {
		e.Wire.Details = b
	}
	return e
}

// errUnknownSite is the same answer as a bad token, so the API is no oracle
// for which sites exist.
func errUnknownSite() *Error {
	return newError(http.StatusUnauthorized, protocol.ErrUnauthorized, "invalid token")
}

func errSyncNotFound() *Error {
	return newError(http.StatusGone, protocol.ErrSyncExpired, "sync session expired or unknown; start a new sync")
}
