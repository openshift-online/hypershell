package main

import (
	"fmt"
	"sort"
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
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return &Spec{Resources: resources, APIPrefix: apiPrefix}, nil
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
