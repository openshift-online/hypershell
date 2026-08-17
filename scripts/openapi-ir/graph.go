package ir

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (normalizer *normalizer) buildSchemaUsesAndGraph() {
	views := make(map[string]*ResourceView)
	for _, operation := range normalizer.document.Operations {
		stream := operationIsStream(operation)
		for _, parameter := range operation.Parameters {
			if parameter.Schema != nil {
				normalizer.addUse(operation.ID, parameter.Schema.Ref, SchemaRoleParameter, parameter.In+":"+parameter.Name)
			}
		}
		if operation.RequestBody != nil {
			for _, mediaType := range operation.RequestBody.Content {
				if mediaType.Schema != nil {
					normalizer.addUse(operation.ID, mediaType.Schema.Ref, SchemaRoleRequest, mediaType.ContentType)
				}
			}
		}
		for _, response := range operation.Responses {
			for _, mediaType := range response.Content {
				if mediaType.Schema == nil {
					continue
				}
				role := SchemaRoleResponse
				if responseIsError(response.Status) {
					role = SchemaRoleError
				} else if stream {
					role = SchemaRoleEvent
				}
				normalizer.addUse(operation.ID, mediaType.Schema.Ref, role, response.Status+":"+mediaType.ContentType)
				if itemRef := normalizer.listItemReference(mediaType.Schema.Ref, make(map[string]bool)); itemRef != "" {
					normalizer.addUse(operation.ID, itemRef, SchemaRoleListItem, response.Status+":"+mediaType.ContentType)
				}
			}
			for _, link := range response.Links {
				normalizer.document.Relationships = append(normalizer.document.Relationships, &Relationship{
					Name: link.Name, SourceOperationID: operation.ID, TargetOperationID: link.TargetOperationID,
					ParameterMappings: append([]ParameterMapping(nil), link.Parameters...),
					Provenance:        RelationshipExplicit, Source: link.Source,
				})
			}
		}

		schemaRef, isList := normalizer.representedSchema(operation)
		kind := resourceKind(operation.Path, isList)
		operation.Capabilities = operationCapabilities(operation, kind, stream)
		if schemaRef == "" {
			continue
		}
		viewPath := operation.Path
		key := viewPath + "\x00" + string(kind) + "\x00" + schemaRef
		view := views[key]
		if view == nil {
			view = &ResourceView{
				ID:   string(kind) + ":" + viewPath + ":" + schemaRef,
				Kind: kind, Path: viewPath, SchemaRef: schemaRef,
				ScopeParameters: scopeParameterNames(operation.PathParameters, kind),
			}
			views[key] = view
			normalizer.document.ResourceViews = append(normalizer.document.ResourceViews, view)
		}
		view.OperationIDs = append(view.OperationIDs, operation.ID)
		view.Capabilities = append(view.Capabilities, operation.Capabilities...)
		if len(view.Extensions) == 0 && len(operation.Extensions) > 0 {
			view.Extensions = operation.Extensions
		}
	}

	for _, view := range normalizer.document.ResourceViews {
		sort.Strings(view.OperationIDs)
		view.Capabilities = sortCapabilities(view.Capabilities)
	}
	for _, operation := range normalizer.document.Operations {
		if operationHasRepresentedSchema(operation) {
			continue
		}
		for _, view := range normalizer.document.ResourceViews {
			if view.Path != operation.Path {
				continue
			}
			view.OperationIDs = append(view.OperationIDs, operation.ID)
			view.Capabilities = sortCapabilities(append(view.Capabilities, operation.Capabilities...))
			sort.Strings(view.OperationIDs)
		}
	}
	normalizer.inferRelationships()
}

func operationHasRepresentedSchema(operation *Operation) bool {
	for _, response := range operation.Responses {
		if !responseIsSuccess(response.Status) {
			continue
		}
		for _, content := range response.Content {
			if content.Schema != nil {
				return true
			}
		}
	}
	if operation.RequestBody != nil {
		for _, content := range operation.RequestBody.Content {
			if content.Schema != nil {
				return true
			}
		}
	}
	return false
}

func (normalizer *normalizer) addUse(operationID, schemaRef string, role SchemaRole, context string) {
	if schemaRef == "" {
		return
	}
	normalizer.document.SchemaUses = append(normalizer.document.SchemaUses, &SchemaUse{
		OperationID: operationID, SchemaRef: schemaRef, Role: role, Context: context,
	})
}

func (normalizer *normalizer) representedSchema(operation *Operation) (string, bool) {
	for _, response := range operation.Responses {
		if !responseIsSuccess(response.Status) {
			continue
		}
		for _, content := range response.Content {
			if content.Schema == nil {
				continue
			}
			if item := normalizer.listItemReference(content.Schema.Ref, make(map[string]bool)); item != "" {
				return item, true
			}
			return content.Schema.Ref, false
		}
	}
	if operation.RequestBody != nil {
		for _, content := range operation.RequestBody.Content {
			if content.Schema != nil {
				return content.Schema.Ref, false
			}
		}
	}
	return "", false
}

func (normalizer *normalizer) listItemReference(ref string, visiting map[string]bool) string {
	if visiting[ref] {
		return ""
	}
	visiting[ref] = true
	defer delete(visiting, ref)
	schema := normalizer.schemas[ref]
	if schema == nil {
		return ""
	}
	if schema.Items != nil {
		return schema.Items.Ref
	}
	properties := normalizer.EffectiveProperties(ref)
	for _, candidate := range []string{"items", "data", "results"} {
		property := properties[candidate]
		if property == nil || property.Schema == nil {
			continue
		}
		propertySchema := normalizer.schemas[property.Schema.Ref]
		if propertySchema != nil && propertySchema.Items != nil {
			return propertySchema.Items.Ref
		}
	}
	for _, composed := range schema.AllOf {
		if item := normalizer.listItemReference(composed.Ref, visiting); item != "" {
			return item
		}
	}
	return ""
}

func (normalizer *normalizer) EffectiveProperties(ref string) map[string]*Property {
	result := make(map[string]*Property)
	normalizer.collectProperties(ref, result, make(map[string]bool))
	return result
}

func (normalizer *normalizer) collectProperties(ref string, result map[string]*Property, visiting map[string]bool) {
	if visiting[ref] {
		return
	}
	visiting[ref] = true
	defer delete(visiting, ref)
	schema := normalizer.schemas[ref]
	if schema == nil {
		return
	}
	for _, composed := range schema.AllOf {
		normalizer.collectProperties(composed.Ref, result, visiting)
	}
	for name, property := range schema.Properties {
		result[name] = property
	}
}

func (document *Document) EffectiveProperties(ref string) map[string]*Property {
	byRef := make(map[string]*Schema, len(document.Schemas))
	for _, schema := range document.Schemas {
		byRef[schema.Ref] = schema
	}
	result := make(map[string]*Property)
	var collect func(string, map[string]bool)
	collect = func(current string, visiting map[string]bool) {
		if visiting[current] {
			return
		}
		visiting[current] = true
		defer delete(visiting, current)
		schema := byRef[current]
		if schema == nil {
			return
		}
		for _, composed := range schema.AllOf {
			collect(composed.Ref, visiting)
		}
		for name, property := range schema.Properties {
			result[name] = property
		}
	}
	collect(ref, make(map[string]bool))
	return result
}

func (normalizer *normalizer) inferRelationships() {
	explicitPairs := make(map[string]bool)
	for _, relationship := range normalizer.document.Relationships {
		explicitPairs[relationship.SourceOperationID+"\x00"+relationship.TargetOperationID] = true
	}
	for _, child := range normalizer.document.ResourceViews {
		if child.Kind != ResourceCollection || len(child.ScopeParameters) == 0 {
			continue
		}
		var candidates []*ResourceView
		for _, parent := range normalizer.document.ResourceViews {
			if parent.Kind != ResourceItem || parent.ID == child.ID {
				continue
			}
			if pathContainsPrefix(child.Path, parent.Path) {
				candidates = append(candidates, parent)
			}
		}
		if len(candidates) != 1 {
			continue
		}
		parent := candidates[0]
		if len(parent.OperationIDs) == 0 || len(child.OperationIDs) == 0 {
			continue
		}
		sourceOperation, targetOperation := parent.OperationIDs[0], child.OperationIDs[0]
		if explicitPairs[sourceOperation+"\x00"+targetOperation] {
			continue
		}
		normalizer.document.Relationships = append(normalizer.document.Relationships, &Relationship{
			Name: "contains", SourceOperationID: sourceOperation, TargetOperationID: targetOperation,
			SourceViewID: parent.ID, TargetViewID: child.ID, Provenance: RelationshipInferred,
		})
	}
}

func operationCapabilities(operation *Operation, kind ResourceViewKind, stream bool) Capabilities {
	var result Capabilities
	if stream {
		result = append(result, CapabilityStream)
	}
	if strings.Contains(operation.Path, ":") {
		result = append(result, CapabilityAction)
		return sortCapabilities(result)
	}
	switch operation.Method {
	case "GET":
		if kind == ResourceCollection {
			result = append(result, CapabilityList)
		} else {
			result = append(result, CapabilityGet)
		}
	case "POST":
		if kind == ResourceCollection {
			result = append(result, CapabilityCreate)
		} else {
			result = append(result, CapabilityAction)
		}
	case "PATCH", "PUT":
		result = append(result, CapabilityUpdate)
	case "DELETE":
		result = append(result, CapabilityDelete)
	default:
		result = append(result, CapabilityAction)
	}
	return sortCapabilities(result)
}

func resourceKind(path string, responseIsList bool) ResourceViewKind {
	if responseIsList {
		return ResourceCollection
	}
	last := path
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		last = path[index+1:]
	}
	if strings.Contains(last, "{") {
		return ResourceItem
	}
	return ResourceCollection
}

func scopeParameterNames(parameters []*Parameter, kind ResourceViewKind) []string {
	length := len(parameters)
	if kind == ResourceItem && length > 0 {
		length--
	}
	result := make([]string, 0, length)
	for _, parameter := range parameters[:length] {
		result = append(result, parameter.Name)
	}
	return result
}

func operationIsStream(operation *Operation) bool {
	for _, response := range operation.Responses {
		for _, content := range response.Content {
			if content.ContentType == "text/event-stream" || content.ContentType == "application/x-ndjson" || content.ContentType == "application/octet-stream" {
				return true
			}
		}
	}
	return false
}

func responseIsSuccess(status string) bool {
	if strings.HasPrefix(status, "2") {
		return true
	}
	value, err := strconv.Atoi(status)
	return err == nil && value >= 200 && value < 300
}

func responseIsError(status string) bool {
	if strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5") || status == "default" {
		return true
	}
	value, err := strconv.Atoi(status)
	return err == nil && value >= 400
}

func pathContainsPrefix(child, parent string) bool {
	if child == parent {
		return false
	}
	parent = strings.TrimSuffix(parent, "/")
	return strings.HasPrefix(child, parent+"/")
}

func (document *Document) ValidateProjectionNames() error {
	for _, schema := range document.Schemas {
		if schema.Name != "" {
			if err := ValidateComponentName(schema.Name); err != nil {
				return fmt.Errorf("schema %s: %w", schema.Ref, err)
			}
		}
		for name := range schema.Properties {
			if err := ValidateIdentifier(name); err != nil {
				return fmt.Errorf("schema %s property: %w", schema.Ref, err)
			}
		}
	}
	for _, operation := range document.Operations {
		if err := ValidateIdentifier(operation.ID); err != nil {
			return fmt.Errorf("operation %s: %w", operation.ID, err)
		}
		for _, segment := range operation.Segments {
			if segment.Literal != "" {
				if err := ValidateRouteLiteral(segment.Literal); err != nil {
					return fmt.Errorf("operation %s: %w", operation.ID, err)
				}
			}
		}
	}
	return nil
}
