package defaultWorkspaceBootstrap

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestBootstrapRecordsASafeSuccessAttempt(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	dao := &fakeDao{}
	provisioner := &fakeProvisioner{}
	service := NewService(dao, provisioner).(*service)
	service.now = func() time.Time { return now }

	if problem := service.Bootstrap(t.Context(), validInput()); problem != nil {
		t.Fatalf("Bootstrap() problem = %#v", problem)
	}
	if len(dao.attempts) != 1 {
		t.Fatalf("attempts = %#v", dao.attempts)
	}
	attempt := dao.attempts[0]
	if attempt.Outcome != OutcomeSucceeded || attempt.LastError != nil || !attempt.AttemptedAt.Equal(now) {
		t.Fatalf("attempt = %#v", attempt)
	}
}

func TestBootstrapRecordsASafeFailureAttempt(t *testing.T) {
	dao := &fakeDao{}
	provisioner := &fakeProvisioner{err: errors.New("client secret super-secret is invalid")}
	service := NewService(dao, provisioner)

	problem := service.Bootstrap(t.Context(), validInput())
	if problem == nil || problem.Status != 503 || problem.Code != "default_workspace_bootstrap_failed" {
		t.Fatalf("Bootstrap() problem = %#v", problem)
	}
	if len(dao.attempts) != 1 || dao.attempts[0].Outcome != OutcomeFailed ||
		dao.attempts[0].LastError == nil || *dao.attempts[0].LastError != "default_workspace_bootstrap_failed" {
		t.Fatalf("attempts = %#v", dao.attempts)
	}
}

func TestBootstrapFailsClosedWhenProvisionerIsUnavailable(t *testing.T) {
	dao := &fakeDao{}
	problem := NewService(dao, nil).Bootstrap(t.Context(), validInput())
	if problem == nil || problem.Status != 503 || problem.Code != "default_workspace_bootstrap_unavailable" {
		t.Fatalf("Bootstrap() problem = %#v", problem)
	}
	if len(dao.attempts) != 0 {
		t.Fatalf("attempts = %#v", dao.attempts)
	}
}

func TestBootstrapReportsUnsupportedControlPlaneAsUnavailable(t *testing.T) {
	service := NewService(&fakeDao{}, &fakeProvisioner{err: errUnsupportedControlPlane})

	problem := service.Bootstrap(t.Context(), validInput())
	if problem == nil || problem.Status != http.StatusServiceUnavailable || problem.Code != "default_workspace_bootstrap_unavailable" {
		t.Fatalf("Bootstrap() problem = %#v, want unavailable", problem)
	}
}

func TestBootstrapSucceedsWhenAuditPersistenceFailsAfterOpenShellSuccess(t *testing.T) {
	dao := &fakeDao{err: errors.New("database unavailable")}
	service := NewService(dao, &fakeProvisioner{})

	if problem := service.Bootstrap(t.Context(), validInput()); problem != nil {
		t.Fatalf("Bootstrap() problem = %#v, want nil", problem)
	}
	if len(dao.attempts) != 0 {
		t.Fatalf("attempts = %#v, want no persisted attempt", dao.attempts)
	}
}

func validInput() Input {
	return Input{
		GatewayID: "gateway-id", GatewayNamespace: "gateway-namespace",
		GatewayEndpoint: "https://gateway.example:443", Issuer: "https://issuer.example/realms/hypershell",
		GatewayClientID: "gateway-client", Subject: "subject-id", SubjectType: SubjectTypeUser, Role: "user",
	}
}

type fakeDao struct {
	attempts []*Attempt
	err      error
}

func (f *fakeDao) Create(_ context.Context, attempt *Attempt) error {
	if f.err != nil {
		return f.err
	}
	f.attempts = append(f.attempts, attempt)
	return nil
}

type fakeProvisioner struct {
	err error
}

func (f *fakeProvisioner) Configured() bool { return true }

func (f *fakeProvisioner) BootstrapDefaultWorkspace(context.Context, Input) error {
	return f.err
}
