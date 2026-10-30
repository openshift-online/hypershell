package ciTrusts

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ciTrustHandler struct {
	service    CITrustService
	generic    services.GenericService
	authorizer *tenancyAuthorizer
}

func NewCITrustHandler(service CITrustService, generic services.GenericService, authorizer *tenancyAuthorizer) *ciTrustHandler {
	return &ciTrustHandler{service: service, generic: generic, authorizer: authorizer}
}

// gatewayIDSearchFilter builds a TSL search clause confining a CITrust listing to
// the given referenced gateways, mirroring the gateways plugin's visibility
// filter so the scope is applied in the query rather than after pagination.
func gatewayIDSearchFilter(gatewayIDs []string) string {
	quoted := make([]string, len(gatewayIDs))
	for i, id := range gatewayIDs {
		quoted[i] = "'" + id + "'"
	}
	return "gateway_id in (" + strings.Join(quoted, ",") + ")"
}

// emptyCITrustList is the zero-item page returned when the caller may read no
// gateway's trusts, so the listing discloses neither items nor a global count.
func emptyCITrustList(page int) openapi.CITrustList {
	kindStr := "CITrustList"
	pageVal := int32(page)
	zero := int32(0)
	return openapi.CITrustList{
		Kind:  &kindStr,
		Page:  &pageVal,
		Size:  &zero,
		Total: &zero,
		Items: []openapi.CITrust{},
	}
}

func (h *ciTrustHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			// Tenancy: confine the listing to the gateways the caller may read
			// (owner/admin/viewer on the referenced gateway, or platform:admin) by
			// scoping the query, so paging and the reported Total stay within the
			// caller's tenancy and one gateway's tenant cannot enumerate another's
			// trusts (SEC-RBAC-22.05).
			scopeIDs, unrestricted, scopeErr := h.authorizer.readableGatewayScope(ctx)
			if scopeErr != nil {
				return nil, scopeErr
			}
			if !unrestricted && len(scopeIDs) == 0 {
				return emptyCITrustList(listArgs.Page), nil
			}
			if !unrestricted {
				filter := gatewayIDSearchFilter(scopeIDs)
				if listArgs.Search != "" {
					listArgs.Search = "(" + listArgs.Search + ") and " + filter
				} else {
					listArgs.Search = filter
				}
			}
			var items []CITrust
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "CITrustList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.CITrustList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.CITrust{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentCITrust(&item))
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, list.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return list, nil
		},
	}
	handlers.HandleList(w, r, cfg)
}

func (h *ciTrustHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Tenancy: a caller with no owner/admin/viewer binding on the referenced
			// gateway (and no platform:admin) is masked as 404 (SEC-RBAC-22).
			if svcErr := h.authorizer.authorizeRead(ctx, item.GatewayID); svcErr != nil {
				return nil, svcErr
			}
			return PresentCITrust(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *ciTrustHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.CITrust
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertCITrust(resource)
			// Tenancy: the caller must manage the gateway the trust references
			// (gateway:owner/admin, or platform:admin) before the trust is created.
			// The gateway id comes from the request body on create (SEC-RBAC-22).
			if svcErr := h.authorizer.authorizeCreate(ctx, item.GatewayID); svcErr != nil {
				return nil, svcErr
			}
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentCITrust(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *ciTrustHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch CITrustPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &patch,
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Tenancy: the caller must manage the gateway the trust references
			// before any mutation. gateway_id is not patchable, so the check on the
			// stored record covers the whole update (SEC-RBAC-22).
			if svcErr := h.authorizer.authorizeManageExisting(ctx, found.GatewayID); svcErr != nil {
				return nil, svcErr
			}
			if patch.SubjectRef != nil {
				found.SubjectRef = patch.SubjectRef
			}
			if patch.SubjectEnvironment != nil {
				found.SubjectEnvironment = patch.SubjectEnvironment
			}
			if patch.SubjectRefProtected != nil {
				found.SubjectRefProtected = patch.SubjectRefProtected
			}
			if patch.Role != nil {
				found.Role = *patch.Role
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentCITrust(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *ciTrustHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Tenancy: the caller must manage the gateway the trust references
			// before the trust is removed (SEC-RBAC-22).
			if svcErr := h.authorizer.authorizeManageExisting(ctx, found.GatewayID); svcErr != nil {
				return nil, svcErr
			}
			if err := h.service.Delete(ctx, id); err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
