package defaultWorkspaceBootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/logger"
)

type Input struct {
	GatewayID        string
	GatewayNamespace string
	GatewayEndpoint  string
	Issuer           string
	GatewayClientID  string
	Subject          string
	SubjectType      string
	Role             string
}

type Provisioner interface {
	Configured() bool
	BootstrapDefaultWorkspace(context.Context, Input) error
}

type Service interface {
	Bootstrap(context.Context, Input) *APIError
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

type service struct {
	dao         Dao
	provisioner Provisioner
	now         func() time.Time
}

const auditPersistenceTimeout = 5 * time.Second

func NewService(dao Dao, provisioner Provisioner) Service {
	return &service{dao: dao, provisioner: provisioner, now: time.Now}
}

func (s *service) Bootstrap(ctx context.Context, input Input) *APIError {
	if err := validate(input); err != nil {
		return &APIError{Status: http.StatusBadRequest, Code: "default_workspace_bootstrap_invalid", Message: err.Error()}
	}
	if s.provisioner == nil || !s.provisioner.Configured() {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_unavailable", Message: "Default-workspace bootstrap is unavailable"}
	}
	bootstrapErr := s.provisioner.BootstrapDefaultWorkspace(ctx, input)
	if errors.Is(bootstrapErr, errUnsupportedControlPlane) {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_unavailable", Message: "Default-workspace bootstrap is unavailable because the control plane does not support it"}
	}
	attempt := &Attempt{
		GatewayID: input.GatewayID, Subject: input.Subject, SubjectType: input.SubjectType,
		Role: input.Role, CorrelationID: logger.GetOperationID(ctx), AttemptedAt: s.now().UTC(),
	}
	if bootstrapErr != nil {
		reason := "default_workspace_bootstrap_failed"
		attempt.Outcome = OutcomeFailed
		attempt.LastError = &reason
	} else {
		attempt.Outcome = OutcomeSucceeded
	}
	auditCtx, cancelAudit := context.WithTimeout(context.Background(), auditPersistenceTimeout)
	defer cancelAudit()
	if auditErr := s.dao.Create(auditCtx, attempt); auditErr != nil {
		if bootstrapErr == nil {
			slog.WarnContext(ctx, "Default-workspace bootstrap succeeded but its audit record could not be persisted",
				"gateway_id", input.GatewayID,
				"subject_type", input.SubjectType,
				"error", auditErr,
			)
			return nil
		}
		return &APIError{Status: http.StatusInternalServerError, Code: "default_workspace_bootstrap_audit_failed", Message: "Default-workspace bootstrap could not be recorded"}
	}
	if bootstrapErr != nil {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "default_workspace_bootstrap_failed", Message: "OpenShell could not add the identity to the default workspace"}
	}
	return nil
}

func validate(input Input) error {
	if strings.TrimSpace(input.GatewayID) == "" || strings.TrimSpace(input.GatewayNamespace) == "" ||
		strings.TrimSpace(input.GatewayEndpoint) == "" || strings.TrimSpace(input.Issuer) == "" ||
		strings.TrimSpace(input.GatewayClientID) == "" || strings.TrimSpace(input.Subject) == "" {
		return errInvalidInput()
	}
	if input.SubjectType != SubjectTypeUser && input.SubjectType != SubjectTypeServiceAccount {
		return errInvalidInput()
	}
	if input.Role != "user" {
		return errInvalidInput()
	}
	return nil
}
