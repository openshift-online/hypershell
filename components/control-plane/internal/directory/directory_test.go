package directory

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
)

type fakeLister struct {
	users []keycloak.RealmUser
	err   error
}

func (f fakeLister) ListRealmUsers(context.Context) ([]keycloak.RealmUser, error) {
	return f.users, f.err
}

func seeded() *Projection {
	p := NewProjection(fakeLister{users: []keycloak.RealmUser{
		{Username: "dana", Name: "Dana Scully", Email: "dana@x"},
		{Username: "dale", Name: "Dale Cooper"},
		{Username: "mulder", Name: "Fox Mulder"},
	}}, 0)
	p.refreshOnce(context.Background())
	return p
}

func TestSearch_SubstringOverUsernameAndName(t *testing.T) {
	p := seeded()
	got := p.Search("da", 50)
	if len(got) != 2 {
		t.Fatalf("Search(da) = %d users, want 2 (dana, dale)", len(got))
	}
	// Name match: "cooper" only matches dale's name.
	if got := p.Search("cooper", 50); len(got) != 1 || got[0].Username != "dale" {
		t.Fatalf("Search(cooper) = %v, want [dale]", got)
	}
}

func TestSearch_LimitCaps(t *testing.T) {
	if got := seeded().Search("", 1); len(got) != 1 {
		t.Fatalf("Search('' , limit 1) = %d, want 1", len(got))
	}
}

func TestResolve_ExactCaseInsensitive(t *testing.T) {
	p := seeded()
	u, ok := p.Resolve("DANA")
	if !ok || u.Email != "dana@x" {
		t.Fatalf("Resolve(DANA) = %v, %v; want dana found", u, ok)
	}
	if _, ok := p.Resolve("ghost"); ok {
		t.Fatal("Resolve(ghost) found a user that is not in the realm")
	}
}

func TestRefresh_KeepsPreviousSnapshotOnError(t *testing.T) {
	p := seeded()
	p.lister = fakeLister{err: errors.New("keycloak down")}
	p.refreshOnce(context.Background())
	if _, ok := p.Resolve("dana"); !ok {
		t.Fatal("a failed refresh must keep the previous snapshot")
	}
}
