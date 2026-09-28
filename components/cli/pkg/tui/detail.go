package tui

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openshift-online/hypershell/components/cli/pkg/gatewayconnect"
)

// toYAML renders a JSON object as block-style YAML, keeping the API's key
// order. JSON is valid YAML, so it parses into a node tree whose flow and
// quoting styles are then cleared.
func toYAML(raw []byte) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("can't format resource: %w", err)
	}
	clearStyle(&doc)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("can't format resource: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("can't format resource: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func clearStyle(n *yaml.Node) {
	n.Style &^= yaml.FlowStyle | yaml.DoubleQuotedStyle | yaml.SingleQuotedStyle
	for _, c := range n.Content {
		clearStyle(c)
	}
}

// detailContent is the full detail text for a resource: its YAML and, for a
// gateway, the openshell connection instructions.
func detailContent(kind Kind, r Resource) string {
	body, err := toYAML(r.Raw)
	if err != nil {
		body = errorStyle.Render(err.Error())
	}
	if kind != KindGateways {
		return body
	}
	var sb strings.Builder
	sb.WriteString(body)
	sb.WriteString("\n\n")
	sb.WriteString(titleStyle.Render("Connect"))
	sb.WriteString("\n\n")
	if r.Field("phase") != "Running" {
		sb.WriteString(mutedStyle.Render("Connection details are available once the gateway is Running."))
		return sb.String()
	}
	var script bytes.Buffer
	if err := gatewayconnect.WriteInstructions(&script, r.Raw); err != nil {
		sb.WriteString(errorStyle.Render(err.Error()))
		return sb.String()
	}
	sb.WriteString(strings.TrimRight(script.String(), "\n"))
	return sb.String()
}
