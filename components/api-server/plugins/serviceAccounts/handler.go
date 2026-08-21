package serviceAccounts

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
)

type handler struct{ service Service }

func NewHandler(service Service) *handler { return &handler{service: service} }

type createRequest struct {
	Name           string     `json:"name"`
	Description    *string    `json:"description"`
	CredentialType string     `json:"credential_type"`
	Role           string     `json:"role"`
	ExpiresAt      *time.Time `json:"expires_at"`
}

func (h *handler) Create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeProblem(w, validationProblem("The request body is not valid JSON or contains an unknown field"))
		return
	}
	vars := mux.Vars(r)
	result, problem := h.service.Create(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), CreateInput(request))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, createResponse{listItemResponse: presentItem(result.Account), Credential: result.Credential})
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	options, problem := parseListOptions(r)
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	vars := mux.Vars(r)
	items, total, access, problem := h.service.List(r.Context(), vars["gateway_id"], rbac.GetUserIDFromContext(r.Context()), options)
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	response := listResponse{Page: options.Page, Size: options.Size, Total: total, Capabilities: presentCapabilities(access), Items: make([]listItemResponse, 0, len(items))}
	for i := range items {
		response.Items = append(response.Items, presentItem(&items[i]))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	account, connection, problem := h.service.Get(r.Context(), vars["gateway_id"], vars["service_account_id"], rbac.GetUserIDFromContext(r.Context()))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	writeJSON(w, http.StatusOK, getResponse{listItemResponse: presentItem(account), Connection: connection})
}

func (h *handler) Revoke(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	account, complete, problem := h.service.Revoke(r.Context(), vars["gateway_id"], vars["service_account_id"], rbac.GetUserIDFromContext(r.Context()))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	status := http.StatusAccepted
	if complete {
		status = http.StatusOK
	}
	writeJSON(w, status, presentItem(account))
}

func (h *handler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	account, complete, problem := h.service.Delete(r.Context(), vars["gateway_id"], vars["service_account_id"], rbac.GetUserIDFromContext(r.Context()))
	if problem != nil {
		writeProblem(w, problem)
		return
	}
	if complete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusAccepted, presentItem(account))
}

func parseListOptions(r *http.Request) (ListOptions, *APIError) {
	query := r.URL.Query()
	options := ListOptions{Page: 1, Size: 20, Sort: "created_at", Order: "desc", Status: query.Get("status"), Search: query.Get("search")}
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
	validStatus := map[string]bool{"": true, StatusProvisioning: true, StatusReady: true, StatusExpired: true, StatusRevoking: true, StatusRevoked: true, StatusDeleting: true, StatusError: true}
	validSort := map[string]bool{"name": true, "role": true, "status": true, "expires_at": true, "created_at": true}
	if !validStatus[options.Status] {
		return ListOptions{}, validationProblem("status is not valid")
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
