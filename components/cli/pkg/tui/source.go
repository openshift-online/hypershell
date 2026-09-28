// Package tui implements `hsctl ui`, the interactive terminal interface for the
// HyperShell API server (specs/platform/hsctl-terminal-ui.spec.md).
package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Kind identifies one of the resource collections the interface shows.
type Kind int

const (
	KindGateways Kind = iota
	KindClusters
	KindReleases
	KindNetworks
)

var kinds = []Kind{KindGateways, KindClusters, KindReleases, KindNetworks}

func (k Kind) Title() string {
	switch k {
	case KindGateways:
		return "Gateways"
	case KindClusters:
		return "Managed Clusters"
	case KindReleases:
		return "Gateway Releases"
	case KindNetworks:
		return "Gateway Networks"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Resource is one API object. Fields holds the decoded JSON object with
// numbers kept as json.Number; Raw holds the body as received so the detail
// view can preserve the API's key order.
type Resource struct {
	ID        string
	Name      string
	CreatedAt time.Time
	Fields    map[string]any
	Raw       json.RawMessage
}

// Field returns a top-level field rendered as display text; absent and null
// fields are empty.
func (r Resource) Field(name string) string {
	v, ok := r.Fields[name]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// DisplayName is the resource name, or its ID when the name is empty.
func (r Resource) DisplayName() string {
	if r.Name != "" {
		return r.Name
	}
	return r.ID
}

// DecodeResource parses one API object.
func DecodeResource(raw []byte) (Resource, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var fields map[string]any
	if err := dec.Decode(&fields); err != nil {
		return Resource{}, fmt.Errorf("can't decode resource: %w", err)
	}
	r := Resource{Fields: fields, Raw: append(json.RawMessage(nil), raw...)}
	r.ID = r.Field("id")
	r.Name = r.Field("name")
	if ts := r.Field("created_at"); ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			r.CreatedAt = t
		}
	}
	return r, nil
}

// sortResources orders resources by case-insensitive name, then ID, so rows do
// not jump between refreshes when the API returns them in a different order.
func sortResources(items []Resource) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].DisplayName()), strings.ToLower(items[j].DisplayName())
		if a != b {
			return a < b
		}
		return items[i].ID < items[j].ID
	})
}

// ListResult is one full refresh of a collection.
type ListResult struct {
	Items []Resource
	// Truncated reports that the page walk stopped at MaxPages while more
	// items remained.
	Truncated bool
}

// GatewayCreate is the only request body the provisioning form sends. Empty
// ClusterID places the gateway on the hub cluster; empty ReleaseID selects the
// platform default release.
type GatewayCreate struct {
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
	ReleaseID string `json:"release_id"`
}

// Source is the application-owned port the interface reads and writes through.
type Source interface {
	List(ctx context.Context, kind Kind) (ListResult, error)
	Get(ctx context.Context, kind Kind, id string) (Resource, error)
	CreateGateway(ctx context.Context, req GatewayCreate) (Resource, error)
	DeleteGateway(ctx context.Context, id string) error
}

// APIError is a non-success API response.
type APIError struct {
	Status int
	Reason string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("API returned %d: %s", e.Status, e.Reason)
	}
	return fmt.Sprintf("API returned %d %s", e.Status, http.StatusText(e.Status))
}

// StatusOf returns the HTTP status of an APIError, or 0 for any other error.
func StatusOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// ReasonOf returns the API error reason when err is an APIError with one. For
// a transport failure it returns the cause without the request URL, which
// would otherwise fill the line; for anything else, err's text.
func ReasonOf(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Reason != "" {
		return apiErr.Reason
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// sentence joins a reason and a follow-up sentence, adding the period the
// reason lacks.
func sentence(reason, next string) string {
	reason = strings.TrimSpace(reason)
	if reason != "" && !strings.ContainsAny(reason[len(reason)-1:], ".!?") {
		reason += "."
	}
	return reason + " " + next
}
