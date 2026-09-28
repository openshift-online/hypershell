package tui

import (
	"bytes"
	"fmt"
	"strconv"
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

// colorizeYAML applies detail-view syntax coloring to block YAML produced by
// toYAML. Indentation and punctuation stay neutral; keys and scalar kinds are
// styled distinctly. Canonical Gateway phase values reuse phaseStyle.
func colorizeYAML(plain string) string {
	if plain == "" {
		return ""
	}
	lines := strings.Split(plain, "\n")
	for i, line := range lines {
		lines[i] = colorizeYAMLLine(line)
	}
	return strings.Join(lines, "\n")
}

func colorizeYAMLLine(line string) string {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	prefix := line[:i]
	rest := line[i:]

	listMarker := ""
	switch {
	case strings.HasPrefix(rest, "- "):
		listMarker = "- "
		rest = rest[2:]
	case rest == "-":
		return prefix + "-"
	}

	key, val, ok := splitYAMLKeyValue(rest)
	if !ok {
		if rest == "" {
			return prefix + listMarker
		}
		return prefix + listMarker + styleYAMLScalar(rest, "")
	}

	styled := yamlKeyStyle.Render(key) + ":"
	if val != "" {
		styled += " " + styleYAMLScalar(val, key)
	}
	return prefix + listMarker + styled
}

// splitYAMLKeyValue splits "key: value" or "key:" on the first colon. Bare
// scalars (no colon) return ok=false.
func splitYAMLKeyValue(s string) (key, val string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return "", "", false
	}
	key = s[:idx]
	rest := s[idx+1:]
	if rest == "" {
		return key, "", true
	}
	if rest[0] == ' ' {
		return key, rest[1:], true
	}
	return key, rest, true
}

func styleYAMLScalar(text, key string) string {
	if key == "phase" {
		switch text {
		case "Running", "Pending", "Provisioning", "Degraded", "Failed":
			return phaseStyle(text).Render(text)
		}
		return yamlStringStyle.Render(text)
	}
	switch text {
	case "null", "~":
		return yamlNullStyle.Render(text)
	case "true", "false":
		return yamlBoolStyle.Render(text)
	}
	if isYAMLNumber(text) {
		return yamlNumberStyle.Render(text)
	}
	return yamlStringStyle.Render(text)
}

func isYAMLNumber(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '"' || s[0] == '\'' {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// detailContent is the full detail text for a resource: its YAML and, for a
// gateway, the openshell connection instructions.
func detailContent(kind Kind, r Resource) string {
	body, err := toYAML(r.Raw)
	if err != nil {
		body = errorStyle.Render(err.Error())
	} else {
		body = colorizeYAML(body)
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
