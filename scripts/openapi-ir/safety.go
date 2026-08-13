package ir

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	componentPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*$`)
	routePartPattern  = regexp.MustCompile(`^[A-Za-z0-9._~:-]+$`)
)

func ValidateIdentifier(value string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("unsafe generated identifier %q", value)
	}
	return nil
}

func ValidateComponentName(value string) error {
	if !componentPattern.MatchString(value) {
		return fmt.Errorf("unsafe OpenAPI component name %q", value)
	}
	return nil
}

func ValidateRouteLiteral(value string) error {
	if !routePartPattern.MatchString(value) {
		return fmt.Errorf("unsafe route literal %q", value)
	}
	return nil
}

func SafeJoin(outputRoot string, elements ...string) (string, error) {
	root, err := filepath.Abs(outputRoot)
	if err != nil {
		return "", fmt.Errorf("resolve output root: %w", err)
	}
	joined := filepath.Join(append([]string{root}, elements...)...)
	relative, err := filepath.Rel(root, joined)
	if err != nil {
		return "", fmt.Errorf("resolve generated output path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("generated path %q escapes output root %q", joined, root)
	}
	return joined, nil
}
