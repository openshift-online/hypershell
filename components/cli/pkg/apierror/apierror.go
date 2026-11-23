// Package apierror turns non-success API responses into CLI errors. A 403 is
// reported as a DeniedError: the caller lacks the role the operation needs, so
// retrying the same command will not help.
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// DeniedError reports an API 403 Forbidden response.
type DeniedError struct {
	// Operation names what was denied, for example "create platform gateway policy".
	Operation string
	// Requires names the role that authorizes the operation, when known.
	Requires string
	// Code and Reason come from the API error body, when present.
	Code   string
	Reason string
}

func (e *DeniedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "permission denied: %s", e.Operation)
	if e.Code != "" || e.Reason != "" {
		b.WriteString(" (API returned 403")
		if e.Code != "" {
			fmt.Fprintf(&b, " %s", e.Code)
		}
		if e.Reason != "" {
			fmt.Fprintf(&b, ": %s", e.Reason)
		}
		b.WriteString(")")
	} else {
		b.WriteString(" (API returned 403)")
	}
	if e.Requires != "" {
		fmt.Fprintf(&b, "; this operation requires %s", e.Requires)
	}
	b.WriteString("; retrying will not succeed until the caller is granted access")
	return b.String()
}

// IsDenied reports whether err is, or wraps, a DeniedError.
func IsDenied(err error) bool {
	var denied *DeniedError
	return errors.As(err, &denied)
}

// Check returns nil when status is one of accepted. A 403 becomes a
// DeniedError naming operation and requires; any other status becomes an error
// carrying the API's code and reason, or the raw body when it has none.
func Check(operation, requires string, status int, body []byte, accepted ...int) error {
	for _, ok := range accepted {
		if status == ok {
			return nil
		}
	}
	var problem struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &problem)
	if status == http.StatusForbidden {
		return &DeniedError{Operation: operation, Requires: requires, Code: problem.Code, Reason: problem.Reason}
	}
	if problem.Reason != "" {
		if problem.Code != "" {
			return fmt.Errorf("can't %s: API returned %d (%s): %s", operation, status, problem.Code, problem.Reason)
		}
		return fmt.Errorf("can't %s: API returned %d: %s", operation, status, problem.Reason)
	}
	return fmt.Errorf("can't %s: API returned %d: %s", operation, status, strings.TrimSpace(string(body)))
}
