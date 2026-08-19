package ir

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type normalizer struct {
	rootPath       string
	raw            *openapi3.T
	scan           *scanner
	document       *Document
	schemas        map[string]*Schema
	schemaIdentity map[*openapi3.Schema]string
	inlineSequence int
}

func normalize(rootPath string, raw *openapi3.T, scan *scanner) (*Document, error) {
	if raw == nil || raw.Info == nil || raw.Paths == nil {
		return nil, newDiagnostic(SourceLocation{File: rootPath}, "document", "OpenAPI document is missing info or paths")
	}
	normalizer := &normalizer{
		rootPath:       rootPath,
		raw:            raw,
		scan:           scan,
		schemas:        make(map[string]*Schema),
		schemaIdentity: make(map[*openapi3.Schema]string),
		document: &Document{
			OpenAPI:    raw.OpenAPI,
			Title:      raw.Info.Title,
			Version:    raw.Info.Version,
			Source:     SourceLocation{File: rootPath, Pointer: ""},
			Extensions: normalizeExtensions(raw.Extensions, SourceLocation{File: rootPath}),
		},
	}
	normalizer.document.Servers = normalizer.normalizeServers(raw.Servers, SourceLocation{File: rootPath, Pointer: "/servers"})
	normalizer.document.Security = normalizeSecurityRequirements(raw.Security)
	normalizer.document.SecuritySchemes = normalizer.normalizeSecuritySchemes()

	if raw.Components != nil {
		names := sortedMapKeys(raw.Components.Schemas)
		for _, name := range names {
			if _, err := normalizer.ensureSchema(raw.Components.Schemas[name], rootPath+"#/components/schemas/"+escapePointer(name), name); err != nil {
				return nil, err
			}
		}
	}
	if err := normalizer.normalizeOperations(); err != nil {
		return nil, err
	}
	normalizer.buildSchemaUsesAndGraph()
	normalizer.finish()
	return normalizer.document, nil
}

func (normalizer *normalizer) normalizeSecuritySchemes() []*SecurityScheme {
	if normalizer.raw.Components == nil {
		return nil
	}
	names := sortedMapKeys(normalizer.raw.Components.SecuritySchemes)
	result := make([]*SecurityScheme, 0, len(names))
	for _, name := range names {
		ref := normalizer.raw.Components.SecuritySchemes[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		value := ref.Value
		source := sourceFromOrigin(value.Origin, normalizer.rootPath, "/components/securitySchemes/"+escapePointer(name))
		result = append(result, &SecurityScheme{
			Name: name, Type: value.Type, Description: value.Description, ParameterName: value.Name,
			In: value.In, Scheme: value.Scheme, BearerFormat: value.BearerFormat,
			OpenIDConnectURL: value.OpenIdConnectUrl,
			Extensions:       normalizeExtensions(value.Extensions, source), Source: source,
		})
	}
	return result
}

func (normalizer *normalizer) normalizeOperations() error {
	paths := make([]string, 0, normalizer.raw.Paths.Len())
	for path := range normalizer.raw.Paths.Map() {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pathItem := normalizer.raw.Paths.Value(path)
		if pathItem == nil {
			continue
		}
		operations := pathItem.Operations()
		methods := make([]string, 0, len(operations))
		for method := range operations {
			methods = append(methods, strings.ToUpper(method))
		}
		sort.Strings(methods)
		for _, method := range methods {
			operation := operations[method]
			if operation == nil {
				operation = operations[strings.ToLower(method)]
			}
			if operation == nil {
				continue
			}
			normalized, err := normalizer.normalizeOperation(path, method, pathItem, operation)
			if err != nil {
				return err
			}
			normalizer.document.Operations = append(normalizer.document.Operations, normalized)
		}
	}
	return nil
}

func (normalizer *normalizer) normalizeOperation(path, method string, pathItem *openapi3.PathItem, raw *openapi3.Operation) (*Operation, error) {
	source := normalizer.scan.sourceForOperation(raw.OperationID)
	if source.File == "" {
		source = sourceFromOrigin(raw.Origin, normalizer.rootPath, "/paths/"+escapePointer(path)+"/"+strings.ToLower(method))
	}
	operation := &Operation{
		ID: raw.OperationID, Method: method, Path: path,
		Tags: append([]string(nil), raw.Tags...), Deprecated: raw.Deprecated,
		Summary: raw.Summary, Description: raw.Description,
		Extensions:     normalizeExtensions(raw.Extensions, source),
		PathExtensions: normalizeExtensions(pathItem.Extensions, sourceFromOrigin(pathItem.Origin, source.File, strings.TrimSuffix(source.Pointer, "/"+strings.ToLower(method)))),
		Source:         source,
	}
	operation.Servers = normalizer.normalizeServers(effectiveServers(raw, pathItem, normalizer.raw.Servers), source)
	operation.Security = normalizeOperationSecurity(raw.Security)

	parameters, err := normalizer.normalizeParameters(pathItem.Parameters, raw.Parameters, source)
	if err != nil {
		return nil, err
	}
	operation.Parameters = parameters
	operation.Segments, operation.PathParameters, err = routeSegments(path, parameters, source, raw.OperationID)
	if err != nil {
		return nil, err
	}

	if raw.RequestBody != nil && raw.RequestBody.Value != nil {
		body := raw.RequestBody.Value
		bodySource := sourceFromOrigin(body.Origin, source.File, source.Pointer+"/requestBody")
		operation.RequestBody = &RequestBody{
			Description: body.Description, Required: body.Required,
			Extensions: normalizeExtensions(body.Extensions, bodySource), Source: bodySource,
		}
		content, err := normalizer.normalizeContent(body.Content, source.File+"#"+source.Pointer+"/requestBody/content")
		if err != nil {
			return nil, err
		}
		operation.RequestBody.Content = content
	}

	if raw.Responses == nil {
		return nil, newDiagnostic(source, "operation "+raw.OperationID, "responses are required")
	}
	statuses := sortedMapKeys(raw.Responses.Map())
	for _, status := range statuses {
		responseRef := raw.Responses.Value(status)
		if responseRef == nil || responseRef.Value == nil {
			return nil, newDiagnostic(source, "operation "+raw.OperationID, "response %s is unresolved", status)
		}
		responseValue := responseRef.Value
		responseSource := sourceFromOrigin(responseValue.Origin, source.File, source.Pointer+"/responses/"+escapePointer(status))
		response := &Response{Status: status, Extensions: normalizeExtensions(responseValue.Extensions, responseSource), Source: responseSource}
		if responseValue.Description != nil {
			response.Description = *responseValue.Description
		}
		content, err := normalizer.normalizeContent(responseValue.Content, source.File+"#"+source.Pointer+"/responses/"+escapePointer(status)+"/content")
		if err != nil {
			return nil, err
		}
		response.Content = content
		response.Links = normalizer.normalizeLinks(responseValue.Links, responseSource)
		operation.Responses = append(operation.Responses, response)
	}
	return operation, nil
}

func (normalizer *normalizer) normalizeParameters(pathParameters, operationParameters openapi3.Parameters, operationSource SourceLocation) ([]*Parameter, error) {
	merged := make([]*openapi3.ParameterRef, 0, len(pathParameters)+len(operationParameters))
	indexes := make(map[string]int)
	for _, parameter := range append(append(openapi3.Parameters(nil), pathParameters...), operationParameters...) {
		if parameter == nil || parameter.Value == nil {
			continue
		}
		key := parameter.Value.In + "\x00" + parameter.Value.Name
		if index, exists := indexes[key]; exists {
			merged[index] = parameter
		} else {
			indexes[key] = len(merged)
			merged = append(merged, parameter)
		}
	}
	result := make([]*Parameter, 0, len(merged))
	for index, reference := range merged {
		raw := reference.Value
		source := sourceFromOrigin(raw.Origin, operationSource.File, fmt.Sprintf("%s/parameters/%d", operationSource.Pointer, index))
		parameter := &Parameter{
			Name: raw.Name, In: raw.In, Description: raw.Description, Required: raw.Required,
			Deprecated: raw.Deprecated, Style: parameterStyle(raw), Explode: parameterExplode(raw),
			AllowReserved: raw.AllowReserved, Example: raw.Example,
			Extensions: normalizeExtensions(raw.Extensions, source), Source: source,
		}
		if raw.Schema != nil {
			ref, err := normalizer.ensureSchema(raw.Schema, operationSource.File+"#"+source.Pointer+"/schema", "")
			if err != nil {
				return nil, err
			}
			parameter.Schema = &SchemaReference{Ref: ref}
		}
		result = append(result, parameter)
	}
	return result, nil
}

func (normalizer *normalizer) normalizeContent(content openapi3.Content, fallback string) ([]*MediaType, error) {
	contentTypes := sortedMapKeys(content)
	result := make([]*MediaType, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		raw := content[contentType]
		if raw == nil {
			continue
		}
		mediaType := &MediaType{ContentType: contentType, Example: raw.Example}
		if raw.Schema != nil {
			ref, err := normalizer.ensureSchema(raw.Schema, fallback+"/"+escapePointer(contentType)+"/schema", "")
			if err != nil {
				return nil, err
			}
			mediaType.Schema = &SchemaReference{Ref: ref}
		}
		result = append(result, mediaType)
	}
	return result, nil
}

func (normalizer *normalizer) normalizeLinks(links openapi3.Links, responseSource SourceLocation) []*OperationLink {
	names := sortedMapKeys(links)
	result := make([]*OperationLink, 0, len(names))
	for _, name := range names {
		ref := links[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		raw := ref.Value
		source := sourceFromOrigin(raw.Origin, responseSource.File, responseSource.Pointer+"/links/"+escapePointer(name))
		link := &OperationLink{
			Name: name, TargetOperationID: raw.OperationID, TargetOperationRef: raw.OperationRef,
			Description: raw.Description, Extensions: normalizeExtensions(raw.Extensions, source), Source: source,
		}
		parameterNames := sortedMapKeys(raw.Parameters)
		for _, parameterName := range parameterNames {
			link.Parameters = append(link.Parameters, ParameterMapping{Target: parameterName, Expression: raw.Parameters[parameterName]})
		}
		result = append(result, link)
	}
	return result
}

func (normalizer *normalizer) ensureSchema(reference *openapi3.SchemaRef, fallback, nameHint string) (string, error) {
	if reference == nil || reference.Value == nil {
		return "", newDiagnostic(sourceFromFallback(fallback), "schema", "unresolved schema reference")
	}
	raw := reference.Value
	if known := normalizer.schemaIdentity[raw]; known != "" {
		return known, nil
	}
	identity, name, source := schemaIdentity(raw, reference.Ref, fallback, nameHint, normalizer.rootPath, &normalizer.inlineSequence)
	if existing := normalizer.schemas[identity]; existing != nil {
		normalizer.schemaIdentity[raw] = identity
		if existing.Name == "" && name != "" {
			existing.Name = name
		}
		return identity, nil
	}
	schema := &Schema{Ref: identity, Name: name, Source: source}
	normalizer.schemas[identity] = schema
	normalizer.schemaIdentity[raw] = identity
	normalizer.document.Schemas = append(normalizer.document.Schemas, schema)

	schema.Types = append([]string(nil), raw.Type.Slice()...)
	schema.Title, schema.Format, schema.Description = raw.Title, raw.Format, raw.Description
	schema.Required = append([]string(nil), raw.Required...)
	sort.Strings(schema.Required)
	schema.Nullable, schema.ReadOnly, schema.WriteOnly, schema.Deprecated = raw.Nullable, raw.ReadOnly, raw.WriteOnly, raw.Deprecated
	schema.Enum = append([]any(nil), raw.Enum...)
	schema.Default, schema.Example = raw.Default, raw.Example
	schema.Minimum, schema.Maximum, schema.ExclusiveMinimum, schema.ExclusiveMaximum = raw.Min, raw.Max, raw.ExclusiveMin, raw.ExclusiveMax
	schema.MultipleOf, schema.MinLength, schema.MaxLength, schema.Pattern = raw.MultipleOf, raw.MinLength, raw.MaxLength, raw.Pattern
	schema.MinItems, schema.MaxItems, schema.UniqueItems = raw.MinItems, raw.MaxItems, raw.UniqueItems
	schema.MinProperties, schema.MaxProperties = raw.MinProps, raw.MaxProps
	schema.Extensions = normalizeExtensions(raw.Extensions, source)
	if raw.Discriminator != nil {
		mapping := make(map[string]string, len(raw.Discriminator.Mapping))
		for key, value := range raw.Discriminator.Mapping {
			mapping[key] = value.Ref
		}
		schema.Discriminator = &Discriminator{PropertyName: raw.Discriminator.PropertyName, Mapping: mapping}
	}

	if raw.Items != nil {
		ref, err := normalizer.ensureSchema(raw.Items, identity+"/items", "")
		if err != nil {
			return "", err
		}
		schema.Items = &SchemaReference{Ref: ref}
	}
	if raw.AdditionalProperties.Schema != nil {
		ref, err := normalizer.ensureSchema(raw.AdditionalProperties.Schema, identity+"/additionalProperties", "")
		if err != nil {
			return "", err
		}
		schema.AdditionalProperties = &SchemaReference{Ref: ref}
	}
	if raw.AdditionalProperties.Has != nil {
		value := *raw.AdditionalProperties.Has
		schema.AdditionalAllowed = &value
	}
	var err error
	if schema.AllOf, err = normalizer.ensureSchemaList(raw.AllOf, identity+"/allOf"); err != nil {
		return "", err
	}
	if schema.OneOf, err = normalizer.ensureSchemaList(raw.OneOf, identity+"/oneOf"); err != nil {
		return "", err
	}
	if schema.AnyOf, err = normalizer.ensureSchemaList(raw.AnyOf, identity+"/anyOf"); err != nil {
		return "", err
	}
	if raw.Not != nil {
		ref, err := normalizer.ensureSchema(raw.Not, identity+"/not", "")
		if err != nil {
			return "", err
		}
		schema.Not = &SchemaReference{Ref: ref}
	}

	if len(raw.Properties) > 0 {
		schema.Properties = make(map[string]*Property, len(raw.Properties))
		required := make(map[string]bool, len(raw.Required))
		for _, name := range raw.Required {
			required[name] = true
		}
		for _, propertyName := range sortedMapKeys(raw.Properties) {
			propertyRef := raw.Properties[propertyName]
			propertyIdentity, err := normalizer.ensureSchema(propertyRef, identity+"/properties/"+escapePointer(propertyName), "")
			if err != nil {
				return "", err
			}
			propertySchema := propertyRef.Value
			propertySource := sourceFromOrigin(propertySchema.Origin, source.File, source.Pointer+"/properties/"+escapePointer(propertyName))
			schema.Properties[propertyName] = &Property{
				Name: propertyName, Schema: &SchemaReference{Ref: propertyIdentity}, Required: required[propertyName],
				ReadOnly: propertySchema.ReadOnly, WriteOnly: propertySchema.WriteOnly, Nullable: propertySchema.Nullable,
				Description: propertySchema.Description, Extensions: normalizeExtensions(propertySchema.Extensions, propertySource), Source: propertySource,
			}
		}
	}
	return identity, nil
}

func (normalizer *normalizer) ensureSchemaList(references openapi3.SchemaRefs, fallback string) ([]*SchemaReference, error) {
	result := make([]*SchemaReference, 0, len(references))
	for index, reference := range references {
		ref, err := normalizer.ensureSchema(reference, fmt.Sprintf("%s/%d", fallback, index), "")
		if err != nil {
			return nil, err
		}
		result = append(result, &SchemaReference{Ref: ref})
	}
	return result, nil
}

func (normalizer *normalizer) normalizeServers(raw openapi3.Servers, source SourceLocation) []*Server {
	result := make([]*Server, 0, len(raw))
	for index, value := range raw {
		if value == nil {
			continue
		}
		serverSource := sourceFromOrigin(value.Origin, source.File, fmt.Sprintf("%s/%d", source.Pointer, index))
		server := &Server{URL: value.URL, Description: value.Description, Source: serverSource, Extensions: normalizeExtensions(value.Extensions, serverSource)}
		if len(value.Variables) > 0 {
			server.Variables = make(map[string]ServerVariable, len(value.Variables))
			for name, variable := range value.Variables {
				if variable != nil {
					server.Variables[name] = ServerVariable{Default: variable.Default, Enum: append([]string(nil), variable.Enum...), Description: variable.Description}
				}
			}
		}
		result = append(result, server)
	}
	return result
}

func (normalizer *normalizer) finish() {
	sort.Slice(normalizer.document.Operations, func(i, j int) bool {
		return normalizer.document.Operations[i].ID < normalizer.document.Operations[j].ID
	})
	sort.Slice(normalizer.document.Schemas, func(i, j int) bool { return normalizer.document.Schemas[i].Ref < normalizer.document.Schemas[j].Ref })
	sort.Slice(normalizer.document.SchemaUses, func(i, j int) bool {
		left, right := normalizer.document.SchemaUses[i], normalizer.document.SchemaUses[j]
		return left.OperationID+"\x00"+string(left.Role)+"\x00"+left.SchemaRef < right.OperationID+"\x00"+string(right.Role)+"\x00"+right.SchemaRef
	})
	sort.Slice(normalizer.document.ResourceViews, func(i, j int) bool {
		return normalizer.document.ResourceViews[i].ID < normalizer.document.ResourceViews[j].ID
	})
	sort.Slice(normalizer.document.Relationships, func(i, j int) bool {
		left, right := normalizer.document.Relationships[i], normalizer.document.Relationships[j]
		if left.Provenance != right.Provenance {
			return left.Provenance == RelationshipExplicit
		}
		return left.SourceOperationID+"\x00"+left.Name+"\x00"+left.TargetOperationID < right.SourceOperationID+"\x00"+right.Name+"\x00"+right.TargetOperationID
	})
}

func normalizeOperationSecurity(raw *openapi3.SecurityRequirements) OperationSecurity {
	if raw == nil {
		return OperationSecurity{State: SecurityInherited}
	}
	if len(*raw) == 0 {
		return OperationSecurity{State: SecurityNone}
	}
	return OperationSecurity{State: SecurityOverride, Requirements: normalizeSecurityRequirements(*raw)}
}

func normalizeSecurityRequirements(raw openapi3.SecurityRequirements) []SecurityRequirement {
	result := make([]SecurityRequirement, 0, len(raw))
	for _, alternative := range raw {
		names := sortedMapKeys(alternative)
		requirement := SecurityRequirement{Schemes: make([]SecuritySchemeUse, 0, len(names))}
		for _, name := range names {
			scopes := append([]string(nil), alternative[name]...)
			sort.Strings(scopes)
			requirement.Schemes = append(requirement.Schemes, SecuritySchemeUse{Name: name, Scopes: scopes})
		}
		result = append(result, requirement)
	}
	return result
}

func effectiveServers(operation *openapi3.Operation, pathItem *openapi3.PathItem, document openapi3.Servers) openapi3.Servers {
	if operation.Servers != nil {
		return *operation.Servers
	}
	if len(pathItem.Servers) > 0 {
		return pathItem.Servers
	}
	return document
}

func routeSegments(path string, parameters []*Parameter, source SourceLocation, operationID string) ([]*RouteSegment, []*Parameter, error) {
	pathParameters := make(map[string]*Parameter)
	for _, parameter := range parameters {
		if parameter.In == openapi3.ParameterInPath {
			pathParameters[parameter.Name] = parameter
		}
	}
	var segments []*RouteSegment
	var ordered []*Parameter
	used := make(map[string]bool)
	for _, slashPart := range strings.Split(strings.Trim(path, "/"), "/") {
		remaining := slashPart
		for remaining != "" {
			start := strings.IndexByte(remaining, '{')
			if start < 0 {
				segments = append(segments, &RouteSegment{Literal: remaining})
				break
			}
			if start > 0 {
				segments = append(segments, &RouteSegment{Literal: remaining[:start]})
			}
			end := strings.IndexByte(remaining[start:], '}')
			if end < 0 {
				return nil, nil, newDiagnostic(source, "operation "+operationID, "path %q has an unterminated parameter", path)
			}
			end += start
			name := remaining[start+1 : end]
			parameter := pathParameters[name]
			if parameter == nil || !parameter.Required {
				return nil, nil, newDiagnostic(source, "operation "+operationID, "path parameter %q is missing or not required", name)
			}
			segments = append(segments, &RouteSegment{Parameter: parameter})
			ordered = append(ordered, parameter)
			used[name] = true
			remaining = remaining[end+1:]
		}
	}
	for name := range pathParameters {
		if !used[name] {
			return nil, nil, newDiagnostic(source, "operation "+operationID, "declared path parameter %q is not present in path %q", name, path)
		}
	}
	return segments, ordered, nil
}

func parameterStyle(parameter *openapi3.Parameter) string {
	if parameter.Style != "" {
		return parameter.Style
	}
	switch parameter.In {
	case openapi3.ParameterInQuery, openapi3.ParameterInCookie:
		return "form"
	default:
		return "simple"
	}
}

func parameterExplode(parameter *openapi3.Parameter) bool {
	if parameter.Explode != nil {
		return *parameter.Explode
	}
	return parameterStyle(parameter) == "form"
}

func schemaIdentity(raw *openapi3.Schema, textualRef, fallback, nameHint, rootPath string, sequence *int) (string, string, SourceLocation) {
	source := sourceFromOrigin(raw.Origin, rootPath, pointerFromFallback(fallback))
	name := nameHint
	if textualRef != "" && raw.Origin != nil && raw.Origin.Key != nil && raw.Origin.Key.Name != "" {
		name = raw.Origin.Key.Name
		identity := canonicalSourceFile(raw.Origin.Key.File, rootPath) + "#/components/schemas/" + escapePointer(name)
		source.File = canonicalSourceFile(raw.Origin.Key.File, rootPath)
		source.Pointer = "/components/schemas/" + escapePointer(name)
		return identity, name, source
	}
	if nameHint != "" && raw.Origin != nil && raw.Origin.Key != nil {
		name = nameHint
		identity := canonicalSourceFile(raw.Origin.Key.File, rootPath) + "#/components/schemas/" + escapePointer(name)
		source.File = canonicalSourceFile(raw.Origin.Key.File, rootPath)
		source.Pointer = "/components/schemas/" + escapePointer(name)
		return identity, name, source
	}
	identity := fallback
	if !strings.Contains(identity, "#") {
		identity = rootPath + "#" + identity
	}
	if identity == "" || strings.HasSuffix(identity, "#") {
		*sequence++
		identity = rootPath + "#/inline/" + strconv.Itoa(*sequence)
	}
	return identity, name, source
}

func sourceFromOrigin(origin *openapi3.Origin, fallbackFile, fallbackPointer string) SourceLocation {
	source := SourceLocation{File: fallbackFile, Pointer: fallbackPointer}
	if origin == nil || origin.Key == nil {
		return source
	}
	if origin.Key.File != "" {
		source.File = canonicalSourceFile(origin.Key.File, fallbackFile)
	}
	source.Line, source.Column = origin.Key.Line, origin.Key.Column
	return source
}

func sourceFromFallback(fallback string) SourceLocation {
	file, pointer := splitCanonicalReference(fallback)
	return SourceLocation{File: file, Pointer: pointer}
}

func pointerFromFallback(fallback string) string {
	_, pointer := splitCanonicalReference(fallback)
	return pointer
}

func canonicalSourceFile(raw, fallback string) string {
	if raw == "" {
		return fallback
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme == "file" {
		raw = parsed.Path
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(fallback), raw)
	}
	absolute, err := filepath.Abs(filepath.FromSlash(raw))
	if err != nil {
		return filepath.Clean(raw)
	}
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		return filepath.Clean(canonical)
	}
	return filepath.Clean(absolute)
}

func normalizeExtensions(raw map[string]any, source SourceLocation) map[string]ExtensionValue {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]ExtensionValue)
	for name, value := range raw {
		if strings.HasPrefix(strings.ToLower(name), "x-") {
			extensionSource := source
			extensionSource.Pointer = strings.TrimSuffix(source.Pointer, "/") + "/" + escapePointer(name)
			result[name] = ExtensionValue{Value: value, Source: extensionSource}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func sortedMapKeys[M ~map[string]V, V any](values M) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
