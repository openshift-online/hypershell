package gatewayAccess

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
)

type handler struct{ service Service }

func NewHandler(service Service) *handler { return &handler{service: service} }

type grantRequest struct {
	Username string `json:"username"`
	Subject  string `json:"subject"`
	Role     string `json:"role"`
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	options, problem := parseListOptions(r)
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	vars := mux.Vars(r)
	items, total, caps, problem := h.service.List(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), options)
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	response := listResponse{Page: options.Page, Size: options.Size, Total: total, Capabilities: presentCapabilities(caps), Items: make([]grantItemResponse, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, presentGrant(item))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) Grant(w http.ResponseWriter, r *http.Request) {
	var request grantRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, validationProblem("The request body is not valid JSON or contains an unknown field"))
		return
	}
	vars := mux.Vars(r)
	item, problem := h.service.Grant(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), GrantInput(request))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	writeJSON(w, http.StatusCreated, presentGrant(item))
}

func (h *handler) ChangeRole(w http.ResponseWriter, r *http.Request) {
	var request changeRoleRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, validationProblem("The request body is not valid JSON or contains an unknown field"))
		return
	}
	vars := mux.Vars(r)
	item, problem := h.service.ChangeRole(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), vars["user_id"], request.Role)
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	writeJSON(w, http.StatusOK, presentGrant(item))
}

func (h *handler) Revoke(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	problem := h.service.Revoke(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), vars["user_id"])
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) SearchDirectory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	candidates, problem := h.service.SearchDirectory(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), r.URL.Query().Get("search"))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	response := directoryResponse{Items: make([]directoryCandidateResponse, 0, len(candidates))}
	for _, c := range candidates {
		response.Items = append(response.Items, presentCandidate(c))
	}
	writeJSON(w, http.StatusOK, response)
}

func parseListOptions(r *http.Request) (ListOptions, *APIError) {
	query := r.URL.Query()
	options := ListOptions{Page: 1, Size: 20, Sort: "granted_at", Order: "desc", Search: query.Get("search"), Role: query.Get("role")}
	var err error
	if value := query.Get("page"); value != "" {
		options.Page, err = strconv.Atoi(value)
		if err != nil || options.Page < 1 {
			return ListOptions{}, validationProblem("page must be an integer greater than zero")
		}
	}
	if value := query.Get("size"); value != "" {
		options.Size, err = strconv.Atoi(value)
		if err != nil || options.Size < 1 || options.Size > 100 {
			return ListOptions{}, validationProblem("size must be between 1 and 100")
		}
	}
	if value := query.Get("sort"); value != "" {
		options.Sort = value
	}
	if value := query.Get("order"); value != "" {
		options.Order = value
	}
	validRole := map[string]bool{"": true, TierOwner: true, TierAdmin: true, TierUser: true}
	validSort := map[string]bool{"granted_at": true, "username": true, "name": true, "role": true}
	if !validRole[options.Role] {
		return ListOptions{}, validationProblem("role filter must be one of owner, admin, user")
	}
	if !validSort[options.Sort] {
		return ListOptions{}, validationProblem("sort is not valid")
	}
	if options.Order != "asc" && options.Order != "desc" {
		return ListOptions{}, validationProblem("order must be asc or desc")
	}
	return options, nil
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeProblem(w http.ResponseWriter, problem *APIError) {
	writeJSON(w, problem.Status, map[string]string{"code": problem.Code, "reason": problem.Message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}
