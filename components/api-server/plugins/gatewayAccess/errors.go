package gatewayAccess

import (
	"fmt"
	"net/http"

	trexerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// APIError is the hand-rolled problem returned by the access facade handlers,
// serialized as {"code","reason"} to match the shared SDK/CLI error contract.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

func validationProblem(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "invalid-request", Message: msg}
}

func forbidden(msg string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func notFound(msg string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not-found", Message: msg}
}

func conflictErr(msg string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: "conflict", Message: msg}
}

func serverError(msg string) *APIError {
	return &APIError{Status: http.StatusInternalServerError, Code: "server-error", Message: msg}
}

func serviceUnavailable(msg string) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "service-unavailable", Message: msg}
}

// fromServiceError maps a trex ServiceError (returned by the shared roleBindings
// service) to the facade's APIError, preserving the HTTP status.
func fromServiceError(serr *trexerrors.ServiceError) *APIError {
	if serr == nil {
		return nil
	}
	status := serr.HttpCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return &APIError{Status: status, Code: fmt.Sprintf("role-binding-%v", serr.Code), Message: serr.Reason}
}
