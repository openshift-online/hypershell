package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	ir "github.com/openshift-online/hypershell/scripts/openapi-ir"
)

func parseSpec(specPath, apiPrefix string) (*Spec, error) {
	document, err := ir.Load(specPath, ir.LoadOptions{})
	if err != nil {
		return nil, fmt.Errorf("load canonical OpenAPI IR: %w", err)
	}
	if err := document.ValidateProjectionNames(); err != nil {
		return nil, fmt.Errorf("validate SDK projection: %w", err)
	}

	resourceViews := primaryCollectionViews(document, apiPrefix)
	resources := make([]Resource, 0, len(resourceViews))
	for _, view := range resourceViews {
		schema := document.Schema(view.SchemaRef)
		if schema == nil || schema.Name == "" {
			continue
		}
		if schema.Name == "ObjectReference" {
			continue
		}
		resource, err := projectResource(document, schema, view)
		if err != nil {
			return nil, fmt.Errorf("project resource %s: %w", schema.Name, err)
		}
		resources = append(resources, resource)
	}
	for _, view := range scopedCollectionViews(document, apiPrefix) {
		resource, err := projectScopedResource(document, view, apiPrefix)
		if err != nil {
			return nil, fmt.Errorf("project scoped resource %s: %w", view.Path, err)
		}
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return &Spec{Resources: resources, APIPrefix: apiPrefix}, nil
}

func projectScopedResource(document *ir.Document, collection *ir.ResourceView, apiPrefix string) (Resource, error) {
	listOperation := operationAt(document, collection.Path, "GET")
	createOperation := operationAt(document, collection.Path, "POST")
	if listOperation == nil || createOperation == nil {
		return Resource{}, fmt.Errorf("scoped collection requires GET and POST operations")
	}

	listType := successSchemaName(document, listOperation, "200")
	createRequestType := requestSchemaName(document, createOperation)
	createResponseType := successSchemaName(document, createOperation, "201")
	if listType == "" || createRequestType == "" || createResponseType == "" {
		return Resource{}, fmt.Errorf("scoped collection requires named list, create request, and create response schemas")
	}
	itemType := listItemSchemaName(document, listType)
	if itemType == "" {
		return Resource{}, fmt.Errorf("list schema %s has no named items schema", listType)
	}

	itemPath := ""
	var getOperation *ir.Operation
	for _, operation := range document.Operations {
		if operation.Method != "GET" || !strings.HasPrefix(operation.Path, strings.TrimSuffix(collection.Path, "/")+"/{") {
			continue
		}
		if strings.Count(strings.TrimPrefix(operation.Path, collection.Path), "/") != 1 {
			continue
		}
		itemPath = operation.Path
		getOperation = operation
		break
	}
	if getOperation == nil {
		return Resource{}, fmt.Errorf("scoped collection has no item GET operation")
	}
	getResponseType := successSchemaName(document, getOperation, "200")
	if getResponseType == "" {
		return Resource{}, fmt.Errorf("item GET requires a named response schema")
	}

	name := strings.TrimSuffix(itemType, "ListItem")
	if name == itemType {
		name = itemType
	}

	scopeParameters := make([]PathParameter, 0, len(collection.ScopeParameters))
	for _, parameter := range collection.ScopeParameters {
		scopeParameters = append(scopeParameters, newPathParameter(parameter))
	}
	itemParameterName := lastPathParameter(itemPath)
	itemParameter := newPathParameter(itemParameterName)

	roots := []string{listType, itemType, createRequestType, createResponseType, getResponseType}
	models := projectModels(document, roots)
	listParameters := make([]Field, 0)
	for _, parameter := range listOperation.Parameters {
		if parameter.In != "query" || parameter.Schema == nil {
			continue
		}
		openAPIType, format, goType, tsType, modelRef := projectedTypes(document, parameter.Schema, parameter.Required, false)
		listParameters = append(listParameters, Field{
			Name: parameter.Name, GoName: toGoName(parameter.Name), TSName: toCamelCase(parameter.Name),
			Type: openAPIType, Format: format, GoType: goType, TSType: tsType,
			Required: parameter.Required, ModelRef: modelRef,
		})
	}

	collectionRelative := relativeAPIPath(collection.Path, apiPrefix)
	itemRelative := relativeAPIPath(itemPath, apiPrefix)
	return Resource{
		Name:               name,
		Plural:             resourcePlural(name),
		PathSegment:        lastLiteralSegment(collection.Path),
		Scoped:             true,
		ScopeParameters:    scopeParameters,
		ItemParameter:      &itemParameter,
		GoCollectionPath:   goPathExpression(collectionRelative, scopeParameters, nil),
		GoItemPath:         goPathExpression(itemRelative, scopeParameters, &itemParameter),
		TSCollectionPath:   tsPathExpression(collectionRelative, scopeParameters, nil),
		TSItemPath:         tsPathExpression(itemRelative, scopeParameters, &itemParameter),
		ListType:           listType,
		ItemType:           itemType,
		CreateRequestType:  createRequestType,
		CreateResponseType: createResponseType,
		GetResponseType:    getResponseType,
		Models:             models,
		ListParameters:     listParameters,
		HasDelete:          operationAt(document, itemPath, "DELETE") != nil,
		Actions:            []string{"revoke"},
	}, nil
}

func scopedCollectionViews(document *ir.Document, apiPrefix string) []*ir.ResourceView {
	var result []*ir.ResourceView
	seen := make(map[string]bool)
	for _, view := range document.ResourceViews {
		if view.Kind != ir.ResourceCollection || !view.Capabilities.Has(ir.CapabilityList) || len(view.ScopeParameters) == 0 {
			continue
		}
		remainder := strings.TrimPrefix(view.Path, strings.TrimSuffix(apiPrefix, "/")+"/")
		if remainder == view.Path || remainder == "" || !strings.Contains(remainder, "/") || seen[view.Path] {
			continue
		}
		seen[view.Path] = true
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func operationAt(document *ir.Document, path, method string) *ir.Operation {
	for _, operation := range document.Operations {
		if operation.Path == path && operation.Method == method {
			return operation
		}
	}
	return nil
}

func successSchemaName(document *ir.Document, operation *ir.Operation, preferredStatus string) string {
	if operation == nil {
		return ""
	}
	for pass := 0; pass < 2; pass++ {
		for _, response := range operation.Responses {
			if pass == 0 && response.Status != preferredStatus {
				continue
			}
			if pass == 1 && !strings.HasPrefix(response.Status, "2") {
				continue
			}
			for _, content := range response.Content {
				if content.Schema != nil {
					if schema := document.Schema(content.Schema.Ref); schema != nil {
						return schema.Name
					}
				}
			}
		}
	}
	return ""
}

func requestSchemaName(document *ir.Document, operation *ir.Operation) string {
	if operation == nil || operation.RequestBody == nil {
		return ""
	}
	for _, content := range operation.RequestBody.Content {
		if content.Schema != nil {
			if schema := document.Schema(content.Schema.Ref); schema != nil {
				return schema.Name
			}
		}
	}
	return ""
}

func listItemSchemaName(document *ir.Document, listType string) string {
	listSchema := document.Schema(listType)
	if listSchema == nil {
		return ""
	}
	items := document.EffectiveProperties(listSchema.Ref)["items"]
	if items == nil || items.Schema == nil {
		return ""
	}
	arraySchema := document.Schema(items.Schema.Ref)
	if arraySchema == nil || arraySchema.Items == nil {
		return ""
	}
	itemSchema := document.Schema(arraySchema.Items.Ref)
	if itemSchema == nil {
		return ""
	}
	return itemSchema.Name
}

func projectModels(document *ir.Document, roots []string) []Model {
	seen := make(map[string]bool)
	var visit func(string)
	visit = func(name string) {
		schema := document.Schema(name)
		if schema == nil || schema.Name == "" || seen[schema.Name] || schema.Name == "ObjectReference" || schema.Name == "Error" {
			return
		}
		seen[schema.Name] = true
		for _, composed := range schema.AllOf {
			if child := document.Schema(composed.Ref); child != nil {
				visit(child.Name)
			}
		}
		for _, property := range document.EffectiveProperties(schema.Ref) {
			visitSchemaReference(document, property.Schema, visit)
		}
		visitSchemaReference(document, schema.Items, visit)
	}
	for _, root := range roots {
		visit(root)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	models := make([]Model, 0, len(names))
	for _, name := range names {
		schema := document.Schema(name)
		model := Model{Name: name}
		if len(schema.Enum) > 0 {
			model.IsEnum = true
			for _, raw := range schema.Enum {
				value := fmt.Sprint(raw)
				model.EnumValues = append(model.EnumValues, EnumValue{Name: enumValueName(name, value), Value: value})
			}
		} else {
			model.Fields = projectModelFields(document, schema.Ref)
		}
		models = append(models, model)
	}

	byName := make(map[string]*Model, len(models))
	for index := range models {
		byName[models[index].Name] = &models[index]
	}
	var containsSensitive func(string, map[string]bool) bool
	containsSensitive = func(name string, visiting map[string]bool) bool {
		if visiting[name] {
			return false
		}
		visiting[name] = true
		defer delete(visiting, name)
		model := byName[name]
		if model == nil {
			return false
		}
		for _, field := range model.Fields {
			if field.Sensitive || (field.ModelRef != "" && containsSensitive(field.ModelRef, visiting)) {
				return true
			}
		}
		return false
	}
	for index := range models {
		models[index].ContainsSensitive = containsSensitive(models[index].Name, make(map[string]bool))
	}
	return models
}

func visitSchemaReference(document *ir.Document, reference *ir.SchemaReference, visit func(string)) {
	if reference == nil {
		return
	}
	schema := document.Schema(reference.Ref)
	if schema == nil {
		return
	}
	if schema.Name != "" {
		visit(schema.Name)
	}
	visitSchemaReference(document, schema.Items, visit)
}

func projectModelFields(document *ir.Document, schemaRef string) []Field {
	properties := document.EffectiveProperties(schemaRef)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]Field, 0, len(names))
	for _, name := range names {
		property := properties[name]
		openAPIType, format, goType, tsType, modelRef := projectedTypes(document, property.Schema, property.Required, property.Nullable)
		fields = append(fields, Field{
			Name: name, GoName: toGoName(name), PythonName: name, TSName: toCamelCase(name),
			Type: openAPIType, Format: format, GoType: goType, PythonType: toPythonType(openAPIType, format), TSType: tsType,
			Required: property.Required, ReadOnly: property.ReadOnly, Nullable: property.Nullable,
			Sensitive: extensionIsTrue(property.Extensions, "x-sensitive"), ModelRef: modelRef,
			JSONTag: jsonTag(name, property.Required),
		})
	}
	return fields
}

func projectedTypes(document *ir.Document, reference *ir.SchemaReference, required, nullable bool) (string, string, string, string, string) {
	schema := document.Schema(reference.Ref)
	if schema == nil {
		return "", "", "any", "unknown", ""
	}
	openAPIType, format := schemaType(schema)
	modelRef := ""
	goType := ""
	tsType := ""
	if schema.Name != "" && (len(schema.Enum) > 0 || len(schema.Properties) > 0 || len(schema.AllOf) > 0) {
		modelRef = schema.Name
		goType = schema.Name
		tsType = schema.Name
	} else if openAPIType == "array" && schema.Items != nil {
		_, _, itemGoType, itemTSType, itemModelRef := projectedTypes(document, schema.Items, true, false)
		goType = "[]" + strings.TrimPrefix(itemGoType, "*")
		tsType = itemTSType + "[]"
		modelRef = itemModelRef
	} else {
		if format == "date-time" {
			goType = "time.Time"
		} else {
			goType = toGoType(openAPIType, format)
		}
		if len(schema.Enum) > 0 {
			values := make([]string, 0, len(schema.Enum))
			for _, value := range schema.Enum {
				values = append(values, strconv.Quote(fmt.Sprint(value)))
			}
			tsType = strings.Join(values, " | ")
		} else {
			tsType = toTSType(openAPIType, format)
		}
	}
	objectModel := modelRef != "" && len(schema.Enum) == 0
	if (nullable || (!required && (format == "date-time" || objectModel))) && !strings.HasPrefix(goType, "[]") {
		goType = "*" + strings.TrimPrefix(goType, "*")
	}
	if nullable {
		tsType += " | null"
	}
	return openAPIType, format, goType, tsType, modelRef
}

func extensionIsTrue(extensions map[string]ir.ExtensionValue, name string) bool {
	extension, ok := extensions[name]
	if !ok {
		return false
	}
	value, ok := extension.Value.(bool)
	return ok && value
}

func newPathParameter(name string) PathParameter {
	goName := toGoName(name)
	return PathParameter{Name: name, GoName: lowerFirst(goName), TSName: toCamelCase(name)}
}

func lastPathParameter(path string) string {
	last := path[strings.LastIndex(path, "/")+1:]
	return strings.Trim(last, "{}")
}

func relativeAPIPath(path, apiPrefix string) string {
	return "/" + strings.TrimPrefix(strings.TrimPrefix(path, strings.TrimSuffix(apiPrefix, "/")), "/")
}

func goPathExpression(path string, scope []PathParameter, item *PathParameter) string {
	format := path
	arguments := make([]string, 0, len(scope)+1)
	for _, parameter := range scope {
		format = strings.ReplaceAll(format, "{"+parameter.Name+"}", "%s")
		arguments = append(arguments, "url.PathEscape("+parameter.GoName+")")
	}
	if item != nil {
		format = strings.ReplaceAll(format, "{"+item.Name+"}", "%s")
		arguments = append(arguments, "url.PathEscape("+item.GoName+")")
	}
	if len(arguments) == 0 {
		return strconv.Quote(format)
	}
	return "fmt.Sprintf(" + strconv.Quote(format) + ", " + strings.Join(arguments, ", ") + ")"
}

func tsPathExpression(path string, scope []PathParameter, item *PathParameter) string {
	result := path
	for _, parameter := range scope {
		result = strings.ReplaceAll(result, "{"+parameter.Name+"}", "${encodeURIComponent("+parameter.TSName+")}")
	}
	if item != nil {
		result = strings.ReplaceAll(result, "{"+item.Name+"}", "${encodeURIComponent("+item.TSName+")}")
	}
	return "`" + result + "`"
}

func projectResource(document *ir.Document, schema *ir.Schema, collection *ir.ResourceView) (Resource, error) {
	fields, required := projectFields(document, schema.Ref, true)
	patchFields, _ := projectFieldsByName(document, schema.Name+"PatchRequest", false)
	statusPatchFields, _ := projectFieldsByName(document, schema.Name+"StatusPatchRequest", false)

	resource := Resource{
		Name: schema.Name, Plural: resourcePlural(schema.Name), PathSegment: lastLiteralSegment(collection.Path),
		Fields: fields, RequiredFields: required, PatchFields: patchFields,
		StatusPatchFields: statusPatchFields, HasStatusPatch: len(statusPatchFields) > 0,
	}
	for _, view := range document.ResourceViews {
		if view.SchemaRef != schema.Ref {
			continue
		}
		resource.HasDelete = resource.HasDelete || view.Capabilities.Has(ir.CapabilityDelete)
		resource.HasPatch = resource.HasPatch || view.Capabilities.Has(ir.CapabilityUpdate)
	}
	for _, operation := range document.Operations {
		if !operation.Capabilities.Has(ir.CapabilityAction) || !strings.Contains(operation.Path, "/"+resource.PathSegment+"/") {
			continue
		}
		action := lastLiteralSegment(operation.Path)
		if colon := strings.LastIndex(action, ":"); colon >= 0 {
			action = action[colon+1:]
		}
		if action != "" && action != "status" {
			resource.Actions = append(resource.Actions, action)
		}
	}
	sort.Strings(resource.Actions)
	return resource, nil
}

func projectFieldsByName(document *ir.Document, name string, includeReadOnly bool) ([]Field, []string) {
	for _, schema := range document.Schemas {
		if schema.Name == name {
			return projectFields(document, schema.Ref, includeReadOnly)
		}
	}
	return nil, nil
}

func projectFields(document *ir.Document, schemaRef string, includeReadOnly bool) ([]Field, []string) {
	properties := document.EffectiveProperties(schemaRef)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var fields []Field
	var required []string
	for _, name := range names {
		property := properties[name]
		if isObjectReferenceField(name) || (!includeReadOnly && property.ReadOnly) {
			continue
		}
		propertySchema := document.Schema(property.Schema.Ref)
		openAPIType, format := schemaType(propertySchema)
		field := Field{
			Name: name, GoName: toGoName(name), PythonName: name, TSName: toCamelCase(name),
			Type: openAPIType, Format: format,
			GoType: toGoType(openAPIType, format), PythonType: toPythonType(openAPIType, format), TSType: toTSType(openAPIType, format),
			Required: property.Required, ReadOnly: property.ReadOnly, JSONTag: jsonTag(name, property.Required),
		}
		if !includeReadOnly {
			field.Required = false
			field.JSONTag = jsonTag(name, false)
		}
		fields = append(fields, field)
		if property.Required {
			required = append(required, name)
		}
	}
	return fields, required
}

func primaryCollectionViews(document *ir.Document, apiPrefix string) []*ir.ResourceView {
	bySchema := make(map[string]*ir.ResourceView)
	for _, view := range document.ResourceViews {
		if view.Kind != ir.ResourceCollection || !view.Capabilities.Has(ir.CapabilityList) {
			continue
		}
		remainder := strings.TrimPrefix(view.Path, strings.TrimSuffix(apiPrefix, "/")+"/")
		if remainder == view.Path || remainder == "" || strings.Contains(remainder, "/") {
			continue
		}
		if current := bySchema[view.SchemaRef]; current == nil || view.Path < current.Path {
			bySchema[view.SchemaRef] = view
		}
	}
	result := make([]*ir.ResourceView, 0, len(bySchema))
	for _, view := range bySchema {
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := document.Schema(result[i].SchemaRef), document.Schema(result[j].SchemaRef)
		return left.Name < right.Name
	})
	return result
}

func schemaType(schema *ir.Schema) (string, string) {
	if schema == nil {
		return "", ""
	}
	typeName := ""
	if len(schema.Types) > 0 {
		typeName = schema.Types[0]
	}
	return typeName, schema.Format
}

func lastLiteralSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		path = path[index+1:]
	}
	return strings.Trim(path, "{}")
}

func inferAPIPrefixFromIR(specPath string) string {
	document, err := ir.Load(specPath, ir.LoadOptions{})
	if err != nil {
		return "/api/v1"
	}
	for _, operation := range document.Operations {
		if strings.HasPrefix(operation.Path, "/api/") {
			parts := strings.Split(operation.Path, "/")
			if len(parts) >= 4 {
				return "/" + parts[1] + "/" + parts[2] + "/" + parts[3]
			}
		}
	}
	return "/api/v1"
}

func resourcePlural(name string) string {
	if strings.HasSuffix(name, "Settings") || strings.HasSuffix(name, "Data") ||
		strings.HasSuffix(name, "Metadata") || strings.HasSuffix(name, "Info") {
		return name
	}
	if strings.HasSuffix(name, "s") {
		return name + "es"
	}
	if strings.HasSuffix(name, "y") {
		prefix := name[:len(name)-1]
		lastChar := name[len(name)-2]
		if lastChar != 'a' && lastChar != 'e' && lastChar != 'i' && lastChar != 'o' && lastChar != 'u' {
			return prefix + "ies"
		}
	}
	return name + "s"
}

var objectReferenceFields = map[string]bool{
	"id": true, "kind": true, "href": true, "created_at": true, "updated_at": true,
}

func isObjectReferenceField(name string) bool {
	return objectReferenceFields[name]
}
