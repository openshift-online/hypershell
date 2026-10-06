package gatewayaccess

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

// CollectionPath is the gateway access facade collection.
func CollectionPath(gatewayID string) string {
	return "/api/hypershell/v1/gateways/" + url.PathEscape(gatewayID) + "/access"
}

// ItemPath is a single user's access on a gateway (keyed by user id).
func ItemPath(gatewayID, userID string) string {
	return CollectionPath(gatewayID) + "/" + url.PathEscape(userID)
}

// Request performs the HTTP call and returns the body. Any status not listed in
// accepted is turned into an error carrying the API's {code,reason} so a 403
// (unauthorized management) or 409 (last-owner protection) surfaces as a
// non-zero exit with a clear message rather than a silent success.
func Request(conn *connection.Connection, method, path string, query url.Values, body io.Reader, accepted ...int) ([]byte, int, error) {
	response, err := conn.Do(method, path, query, body)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("can't read API response: %w", err)
	}
	for _, status := range accepted {
		if response.StatusCode == status {
			return responseBody, response.StatusCode, nil
		}
	}
	var problem struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(responseBody, &problem)
	if problem.Reason != "" {
		return nil, response.StatusCode, fmt.Errorf("API returned %d (%s): %s", response.StatusCode, problem.Code, problem.Reason)
	}
	return nil, response.StatusCode, fmt.Errorf("API returned %d", response.StatusCode)
}

// ValidateRole enforces the console tier vocabulary.
func ValidateRole(value string) error {
	if value != "owner" && value != "admin" && value != "user" {
		return fmt.Errorf("--role must be one of owner, admin, or user")
	}
	return nil
}

// ValidateOutput mirrors the service-account CLI: only json output is supported.
func ValidateOutput(value string) error {
	if value != "json" {
		return fmt.Errorf("--output must be json")
	}
	return nil
}

// WriteJSON pretty-prints the API response (or a small synthetic object) to the
// writer.
func WriteJSON(writer io.Writer, body []byte) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("API returned invalid JSON: %w", err)
	}
	var rendered bytes.Buffer
	encoder := json.NewEncoder(&rendered)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("can't encode output: %w", err)
	}
	_, err := writer.Write(rendered.Bytes())
	return err
}

// EmptyJSON is returned for 204 responses that carry no body.
func EmptyJSON(status, userID string) []byte {
	body, _ := json.Marshal(map[string]string{"user_id": userID, "status": status})
	return body
}
