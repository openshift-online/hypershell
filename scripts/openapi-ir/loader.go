package ir

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

type LoadOptions struct {
	AllowedRoots                []string
	AllowAbsoluteFileReferences bool
}

type scannedDocument struct {
	path string
	root *yaml.Node
}

type referenceEdge struct {
	from     string
	to       string
	source   SourceLocation
	isSchema bool
}

type operationLocation struct {
	id     string
	source SourceLocation
}

type scanner struct {
	options      LoadOptions
	roots        []string
	documents    map[string]*scannedDocument
	edges        []referenceEdge
	operations   []operationLocation
	allowedFiles map[string]struct{}
}

func Load(specPath string, options LoadOptions) (*Document, error) {
	rootPath, err := filepath.Abs(specPath)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAPI path: %w", err)
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAPI path: %w", err)
	}

	scan, err := newScanner(rootPath, options)
	if err != nil {
		return nil, err
	}
	if err := scan.scanFile(rootPath); err != nil {
		return nil, err
	}
	if err := scan.validateReferences(); err != nil {
		return nil, err
	}
	if err := scan.validateOperationIDs(); err != nil {
		return nil, err
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.IncludeOrigin = true
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
		path, pathErr := scan.canonicalURIPath(location)
		if pathErr != nil {
			return nil, pathErr
		}
		if _, ok := scan.allowedFiles[path]; !ok {
			return nil, fmt.Errorf("reference target %q was not approved by the bounded preflight", path)
		}
		return os.ReadFile(path)
	}

	raw, err := loader.LoadFromFile(rootPath)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI %s: %w", rootPath, err)
	}

	document, err := normalize(rootPath, raw, scan)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func newScanner(rootPath string, options LoadOptions) (*scanner, error) {
	configured := append([]string(nil), options.AllowedRoots...)
	if len(configured) == 0 {
		configured = []string{filepath.Dir(rootPath)}
	}
	roots := make([]string, 0, len(configured))
	for _, configuredRoot := range configured {
		absolute, err := filepath.Abs(configuredRoot)
		if err != nil {
			return nil, fmt.Errorf("resolve document root %q: %w", configuredRoot, err)
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("resolve document root %q: %w", configuredRoot, err)
		}
		roots = append(roots, filepath.Clean(canonical))
	}
	sort.Strings(roots)
	return &scanner{
		options:      options,
		roots:        roots,
		documents:    make(map[string]*scannedDocument),
		allowedFiles: make(map[string]struct{}),
	}, nil
}

func (scan *scanner) scanFile(path string) error {
	path = filepath.Clean(path)
	if _, loaded := scan.documents[path]; loaded {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return newDiagnostic(SourceLocation{File: path}, "reference", "read target: %v", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return newDiagnostic(SourceLocation{File: path}, "document", "parse YAML: %v", err)
	}
	scan.documents[path] = &scannedDocument{path: path, root: &root}
	scan.allowedFiles[path] = struct{}{}
	return scan.walk(path, documentNode(&root), "")
}

func (scan *scanner) walk(path string, node *yaml.Node, pointer string) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		if isPathItemPointer(pointer) {
			if err := scan.collectOperations(path, node, pointer); err != nil {
				return err
			}
		}
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			childPointer := pointer + "/" + escapePointer(key.Value)
			if key.Value == "$ref" && value.Kind == yaml.ScalarNode {
				if err := scan.collectReference(path, pointer, value); err != nil {
					return err
				}
				continue
			}
			if err := scan.walk(path, value, childPointer); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, child := range node.Content {
			if err := scan.walk(path, child, fmt.Sprintf("%s/%d", pointer, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (scan *scanner) collectReference(referringFile, pointer string, value *yaml.Node) error {
	targetFile, fragment, err := scan.resolveReference(referringFile, value.Value, SourceLocation{
		File: referringFile, Pointer: pointer, Line: value.Line, Column: value.Column,
	})
	if err != nil {
		return err
	}
	if err := scan.scanFile(targetFile); err != nil {
		return err
	}
	targetPointer := fragment
	if targetPointer == "" {
		targetPointer = ""
	}
	scan.edges = append(scan.edges, referenceEdge{
		from:     referringFile + "#" + pointer,
		to:       targetFile + "#" + targetPointer,
		source:   SourceLocation{File: referringFile, Pointer: pointer, Line: value.Line, Column: value.Column},
		isSchema: strings.Contains(pointer, "/schemas/") && strings.Contains(targetPointer, "/schemas/"),
	})
	return nil
}

func (scan *scanner) resolveReference(referringFile, raw string, source SourceLocation) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", newDiagnostic(source, "reference", "invalid $ref %q: %v", raw, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return "", "", newDiagnostic(source, "reference", "non-file URI reference %q is not allowed", raw)
	}
	refPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", "", newDiagnostic(source, "reference", "invalid escaped path in $ref %q", raw)
	}
	if filepath.IsAbs(refPath) && !scan.options.AllowAbsoluteFileReferences {
		return "", "", newDiagnostic(source, "reference", "absolute file reference %q is not allowed", raw)
	}
	target := referringFile
	if refPath != "" {
		if filepath.IsAbs(refPath) {
			target = filepath.Clean(refPath)
		} else {
			target = filepath.Join(filepath.Dir(referringFile), filepath.FromSlash(refPath))
		}
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", newDiagnostic(source, "reference", "resolve target %q: %v", raw, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", newDiagnostic(source, "reference", "resolve target %q: %v", raw, err)
	}
	if !scan.withinRoots(canonical) {
		return "", "", newDiagnostic(source, "reference", "target %q resolves outside configured document roots", raw)
	}
	fragment := parsed.Fragment
	if fragment != "" {
		decoded, decodeErr := url.PathUnescape(fragment)
		if decodeErr != nil {
			return "", "", newDiagnostic(source, "reference", "invalid fragment in $ref %q", raw)
		}
		if !strings.HasPrefix(decoded, "/") {
			return "", "", newDiagnostic(source, "reference", "fragment in $ref %q is not a JSON Pointer", raw)
		}
		fragment = decoded
	}
	return filepath.Clean(canonical), fragment, nil
}

func (scan *scanner) withinRoots(target string) bool {
	for _, root := range scan.roots {
		relative, err := filepath.Rel(root, target)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func (scan *scanner) canonicalURIPath(location *url.URL) (string, error) {
	if location == nil || location.Scheme != "" || location.Host != "" {
		return "", fmt.Errorf("only bounded local file references are supported: %v", location)
	}
	path, err := url.PathUnescape(location.Path)
	if err != nil {
		return "", fmt.Errorf("decode reference path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(filepath.FromSlash(path))
	if err != nil {
		return "", fmt.Errorf("resolve reference path: %w", err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", err
	}
	if !scan.withinRoots(canonical) {
		return "", fmt.Errorf("reference target %q is outside configured document roots", canonical)
	}
	return filepath.Clean(canonical), nil
}

func (scan *scanner) collectOperations(path string, pathItem *yaml.Node, pointer string) error {
	for index := 0; index+1 < len(pathItem.Content); index += 2 {
		methodNode := pathItem.Content[index]
		operationNode := pathItem.Content[index+1]
		method := strings.ToLower(methodNode.Value)
		if !isHTTPMethod(method) || operationNode.Kind != yaml.MappingNode {
			continue
		}
		operationPointer := pointer + "/" + method
		operationID := mappingValue(operationNode, "operationId")
		if operationID == nil || strings.TrimSpace(operationID.Value) == "" {
			return newDiagnostic(SourceLocation{File: path, Pointer: operationPointer, Line: methodNode.Line, Column: methodNode.Column}, method+" operation", "missing required operationId")
		}
		scan.operations = append(scan.operations, operationLocation{
			id:     operationID.Value,
			source: SourceLocation{File: path, Pointer: operationPointer, Line: operationID.Line, Column: operationID.Column},
		})
	}
	return nil
}

func (scan *scanner) validateOperationIDs() error {
	seen := make(map[string]SourceLocation)
	for _, operation := range scan.operations {
		if first, duplicate := seen[operation.id]; duplicate {
			return newDiagnostic(operation.source, "operation "+operation.id, "duplicate operationId; first declared at %s#%s", first.File, first.Pointer)
		}
		seen[operation.id] = operation.source
	}
	return nil
}

func (scan *scanner) validateReferences() error {
	for _, edge := range scan.edges {
		file, pointer := splitCanonicalReference(edge.to)
		document := scan.documents[file]
		if document == nil || findPointer(document.root, pointer) == nil {
			return newDiagnostic(edge.source, "reference", "unresolved target %s", edge.to)
		}
	}

	bySource := make(map[string][]referenceEdge)
	for _, edge := range scan.edges {
		bySource[edge.from] = append(bySource[edge.from], edge)
	}
	state := make(map[string]int)
	var visit func(string, []referenceEdge) error
	visit = func(node string, path []referenceEdge) error {
		state[node] = 1
		for _, edge := range bySource[node] {
			if state[edge.to] == 1 {
				allSchema := edge.isSchema
				for _, prior := range path {
					allSchema = allSchema && prior.isSchema
				}
				if !allSchema {
					return newDiagnostic(edge.source, "reference", "cyclic non-schema reference ending at %s", edge.to)
				}
				continue
			}
			if state[edge.to] == 0 {
				if err := visit(edge.to, append(path, edge)); err != nil {
					return err
				}
			}
		}
		state[node] = 2
		return nil
	}
	for source := range bySource {
		if state[source] == 0 {
			if err := visit(source, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (scan *scanner) sourceForOperation(id string) SourceLocation {
	for _, operation := range scan.operations {
		if operation.id == id {
			return operation.source
		}
	}
	return SourceLocation{}
}

func documentNode(root *yaml.Node) *yaml.Node {
	if root != nil && root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func mappingValue(mapping *yaml.Node, name string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == name {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func findPointer(root *yaml.Node, pointer string) *yaml.Node {
	node := documentNode(root)
	if pointer == "" {
		return node
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil
	}
	for _, token := range strings.Split(pointer[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		if node == nil || node.Kind != yaml.MappingNode {
			return nil
		}
		node = mappingValue(node, token)
	}
	return node
}

func splitCanonicalReference(reference string) (string, string) {
	index := strings.LastIndex(reference, "#")
	if index < 0 {
		return reference, ""
	}
	return reference[:index], reference[index+1:]
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func isPathItemPointer(pointer string) bool {
	if !strings.HasPrefix(pointer, "/paths/") {
		return false
	}
	return !strings.Contains(strings.TrimPrefix(pointer, "/paths/"), "/")
}

func isHTTPMethod(method string) bool {
	switch method {
	case "connect", "delete", "get", "head", "options", "patch", "post", "put", "trace":
		return true
	default:
		return false
	}
}
