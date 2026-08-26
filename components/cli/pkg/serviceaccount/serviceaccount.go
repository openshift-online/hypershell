package serviceaccount

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

const WorkspaceMembershipNote = "Service accounts do not inherit the creator's OpenShell workspaces. An openshell-user subject requires a separate workspace membership granted by an OpenShell gateway administrator."

func CollectionPath(gatewayID string) string {
	return "/api/hypershell/v1/gateways/" + url.PathEscape(gatewayID) + "/service_accounts"
}

func ItemPath(gatewayID, serviceAccountID string) string {
	return CollectionPath(gatewayID) + "/" + url.PathEscape(serviceAccountID)
}

func RevokePath(gatewayID, serviceAccountID string) string {
	return ItemPath(gatewayID, serviceAccountID) + "/revoke"
}

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

// ReserveOutput exclusively creates the mode-0600 output file so the target is
// secured before any credential is requested. It returns a nil handle when the
// output goes to stdout (empty path or "-"), in which case no reservation is
// needed. The returned handle must be passed to WriteReserved, or removed with
// ReleaseOutput if the credential is never written.
func ReserveOutput(outputFile string) (*os.File, error) {
	if outputFile == "" || outputFile == "-" {
		return nil, nil
	}
	file, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("can't create output file %q: %w", outputFile, err)
	}
	return file, nil
}

// ReleaseOutput closes and removes a reservation created by ReserveOutput. It is
// used when the credential is never produced (for example the API request
// failed) so the empty reservation does not block a retry.
func ReleaseOutput(file *os.File) {
	if file == nil {
		return
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
}

// WriteReserved renders the structured credential bundle to the reserved file
// handle, or to writer when file is nil (stdout output). The file handle is
// closed before returning.
func WriteReserved(writer io.Writer, file *os.File, body []byte) error {
	rendered, err := renderStructured(body)
	if err != nil {
		return err
	}
	if file == nil {
		_, err := writer.Write(rendered)
		return err
	}
	defer file.Close()
	if _, err := file.Write(rendered); err != nil {
		return fmt.Errorf("can't write output file %q: %w", file.Name(), err)
	}
	return file.Sync()
}

// WriteStructured reserves the output target and writes the credential bundle in
// a single step. Callers that must secure the output before requesting the
// credential should use ReserveOutput and WriteReserved instead.
func WriteStructured(writer io.Writer, outputFile string, body []byte) error {
	file, err := ReserveOutput(outputFile)
	if err != nil {
		return err
	}
	return WriteReserved(writer, file, body)
}

func renderStructured(body []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("API returned invalid JSON: %w", err)
	}
	if object, ok := value.(map[string]any); ok {
		object["workspace_membership_note"] = WorkspaceMembershipNote
	}
	var rendered bytes.Buffer
	encoder := json.NewEncoder(&rendered)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("can't encode output: %w", err)
	}
	return rendered.Bytes(), nil
}

func ValidateOutput(value string) error {
	if value != "json" {
		return fmt.Errorf("--output must be json")
	}
	return nil
}

func ValidateRole(value string) error {
	if value != "openshell-user" && value != "openshell-admin" {
		return fmt.Errorf("--role must be openshell-user or openshell-admin")
	}
	return nil
}

func Expiration(expiresAt, expiresIn string, now time.Time) (string, error) {
	if expiresAt != "" && expiresIn != "" {
		return "", fmt.Errorf("--expires-at and --expires-in are mutually exclusive")
	}
	if expiresAt != "" {
		value, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return "", fmt.Errorf("--expires-at must be RFC 3339: %w", err)
		}
		return value.UTC().Format(time.RFC3339), nil
	}
	if expiresIn == "" {
		return "", nil
	}
	duration, err := parseDuration(expiresIn)
	if err != nil || duration <= 0 {
		return "", fmt.Errorf("--expires-in must be a positive duration such as 24h or 30d")
	}
	return now.UTC().Add(duration).Format(time.RFC3339), nil
}

func parseDuration(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil {
			return 0, err
		}
		if days < 1 || days > 365 {
			return 0, fmt.Errorf("day duration must be between 1d and 365d")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func EmptyJSON(status string, id string) []byte {
	body, _ := json.Marshal(map[string]string{"id": id, "status": status})
	return body
}
