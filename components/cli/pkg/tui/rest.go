package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
	"github.com/openshift-online/hypershell/components/cli/pkg/urls"
)

const (
	// PageSize is the list page size each refresh requests.
	PageSize = 100
	// MaxPages bounds one refresh's page walk.
	MaxPages = 50
)

// RESTSource implements Source over the HyperShell REST API.
type RESTSource struct {
	conn *connection.Connection
}

func NewRESTSource(conn *connection.Connection) *RESTSource {
	return &RESTSource{conn: conn}
}

func collectionPath(kind Kind) string {
	switch kind {
	case KindClusters:
		return urls.ManagedClustersPath
	}
	return urls.GatewaysPath
}

type listPage struct {
	Total int               `json:"total"`
	Items []json.RawMessage `json:"items"`
}

func (s *RESTSource) List(ctx context.Context, kind Kind) (ListResult, error) {
	var result ListResult
	for page := 1; page <= MaxPages; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("size", strconv.Itoa(PageSize))
		body, err := s.do(ctx, http.MethodGet, collectionPath(kind), query, nil, http.StatusOK)
		if err != nil {
			return ListResult{}, err
		}
		var lp listPage
		if err := json.Unmarshal(body, &lp); err != nil {
			return ListResult{}, fmt.Errorf("can't decode %s list: %w", strings.ToLower(kind.Title()), err)
		}
		for _, raw := range lp.Items {
			r, err := DecodeResource(raw)
			if err != nil {
				return ListResult{}, err
			}
			result.Items = append(result.Items, r)
		}
		if len(lp.Items) < PageSize {
			return result, nil
		}
		if page == MaxPages {
			// A full last page means more may remain; the total, when the API
			// reports one, settles it.
			result.Truncated = lp.Total == 0 || lp.Total > len(result.Items)
		}
	}
	return result, nil
}

func (s *RESTSource) Get(ctx context.Context, kind Kind, id string) (Resource, error) {
	body, err := s.do(ctx, http.MethodGet, collectionPath(kind)+"/"+url.PathEscape(id), nil, nil, http.StatusOK)
	if err != nil {
		return Resource{}, err
	}
	return DecodeResource(body)
}

func (s *RESTSource) CreateGateway(ctx context.Context, req GatewayCreate) (Resource, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return Resource{}, fmt.Errorf("can't encode gateway: %w", err)
	}
	body, err := s.do(ctx, http.MethodPost, urls.GatewaysPath, nil, payload, http.StatusCreated)
	if err != nil {
		return Resource{}, err
	}
	return DecodeResource(body)
}

func (s *RESTSource) DeleteGateway(ctx context.Context, id string) error {
	_, err := s.do(ctx, http.MethodDelete, urls.GatewayPath(url.PathEscape(id)), nil, nil, http.StatusNoContent)
	return err
}

func (s *RESTSource) do(ctx context.Context, method, path string, query url.Values, payload []byte, want int) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	resp, err := s.conn.DoContext(ctx, method, path, query, reader)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("can't read response: %w", err)
	}
	if resp.StatusCode != want {
		return nil, &APIError{Status: resp.StatusCode, Reason: errorReason(body)}
	}
	return body, nil
}

// errorReason extracts the `reason` of an API Error body, falling back to the
// trimmed body text.
func errorReason(body []byte) string {
	var apiErr struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Reason != "" {
		return apiErr.Reason
	}
	text := strings.TrimSpace(string(body))
	const limit = 200
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	return text
}
