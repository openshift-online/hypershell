package extsearch

import (
	"context"
	"testing"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// recordingService is a stand-in inner GenericService that records whether List
// was delegated and with which search, so a test can prove the wrapper forwarded
// the call rather than short-circuiting it.
type recordingService struct {
	called bool
	search string
}

func (r *recordingService) List(ctx context.Context, username string, args *services.ListArguments, resourceList interface{}) (*api.PagingMeta, *errors.ServiceError) {
	r.called = true
	if args != nil {
		r.search = args.Search
	}
	return &api.PagingMeta{}, nil
}

// TestListFailsOpenOnUnclassifiableShape is the fail-open regression for FINDING
// 5. When the resource list is not a *[]T its element type cannot be reflected for
// search-column validation. The wrapper must DELEGATE the List unchanged (search
// validation is best-effort defense) rather than turn an otherwise-valid request
// into a 500. Before the fix, newModelPointer returned a GeneralError (500) for
// any non-*[]T shape, so this test would fail with a non-nil error and no
// delegation.
func TestListFailsOpenOnUnclassifiableShape(t *testing.T) {
	inner := &recordingService{}
	svc := NewValidatingService(inner)

	// A shape the reflection path cannot classify as *[]T: a plain pointer to a
	// struct. The request carries a non-empty search, which is exactly the path
	// that previously reached the 500 branch.
	notASlice := &struct{ Name string }{}
	args := &services.ListArguments{Search: "name = 'x'"}

	_, serr := svc.List(context.Background(), "someone", args, notASlice)
	if serr != nil {
		t.Fatalf("List should fail open (delegate) on an unclassifiable shape, got error: %v", serr)
	}
	if !inner.called {
		t.Fatal("List should have delegated to the inner service on an unclassifiable shape")
	}
	if inner.search != "name = 'x'" {
		t.Fatalf("delegated search mutated: got %q", inner.search)
	}
}

// TestListValidatesClassifiableShape confirms the fail-open change did not disable
// validation on the normal *[]T path: an unknown search column on a classifiable
// list is still rejected with a clean 400 before SQL.
func TestListValidatesClassifiableShape(t *testing.T) {
	inner := &recordingService{}
	svc := NewValidatingService(inner)

	var list []model
	args := &services.ListArguments{Search: "definitely_not_a_column = 'x'"}

	_, serr := svc.List(context.Background(), "someone", args, &list)
	if serr == nil {
		t.Fatal("List should reject an unknown search column on a classifiable list")
	}
	if inner.called {
		t.Fatal("List must not delegate when the search column is invalid")
	}
}
