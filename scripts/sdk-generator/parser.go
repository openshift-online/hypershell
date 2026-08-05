package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type openAPIDoc struct {
	Info struct {
		Title string `yaml:"title"`
	} `yaml:"info"`
	Paths      map[string]interface{} `yaml:"paths"`
	Components struct {
		Schemas map[string]interface{} `yaml:"schemas"`
	} `yaml:"components"`
}

type subSpecDoc struct {
	Paths      map[string]interface{} `yaml:"paths"`
	Components struct {
		Schemas map[string]interface{} `yaml:"schemas"`
	} `yaml:"components"`
}

func parseSpec(specPath, apiPrefix string) (*Spec, error) {
	mainData, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var mainDoc openAPIDoc
	if err := yaml.Unmarshal(mainData, &mainDoc); err != nil {
		return nil, fmt.Errorf("parse main spec: %w", err)
	}

	specDir := filepath.Dir(specPath)

	resourceSpecs, err := discoverResources(specDir, &mainDoc, apiPrefix)
	if err != nil {
		return nil, fmt.Errorf("discover resources: %w", err)
	}

	var resources []Resource
	for name, info := range resourceSpecs {
		resource, err := extractResource(name, info.pathSegment, info.doc)
		if err != nil {
			return nil, fmt.Errorf("extract resource %s: %w", name, err)
		}
		resources = append(resources, *resource)
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	return &Spec{Resources: resources, APIPrefix: apiPrefix}, nil
}

type resourceInfo struct {
	pathSegment string
	doc         *subSpecDoc
}

func discoverResources(specDir string, mainDoc *openAPIDoc, apiPrefix string) (map[string]resourceInfo, error) {
	discovered := make(map[string]resourceInfo)
	loadedFiles := make(map[string]*subSpecDoc)

	for schemaName, schemaVal := range mainDoc.Components.Schemas {
		if strings.HasSuffix(schemaName, "List") || strings.HasSuffix(schemaName, "PatchRequest") ||
			strings.HasSuffix(schemaName, "StatusPatchRequest") {
			continue
		}

		if schemaName == "ObjectReference" || schemaName == "List" || schemaName == "Error" {
			continue
		}

		refMap, ok := schemaVal.(map[string]interface{})
		if !ok {
			continue
		}

		refStr, ok := refMap["$ref"].(string)
		if !ok {
			continue
		}

		parts := strings.SplitN(refStr, "#", 2)
		if len(parts) != 2 {
			continue
		}

		subFile := parts[0]
		subPath := filepath.Join(specDir, subFile)

		subDoc, loaded := loadedFiles[subFile]
		if !loaded {
			data, err := os.ReadFile(subPath)
			if err != nil {
				return nil, fmt.Errorf("read sub-spec %s: %w", subFile, err)
			}
			subDoc = &subSpecDoc{}
			if err := yaml.Unmarshal(data, subDoc); err != nil {
				return nil, fmt.Errorf("parse sub-spec %s: %w", subFile, err)
			}
			loadedFiles[subFile] = subDoc
		}

		pathSegment := inferPathSegment(mainDoc.Paths, apiPrefix, schemaName)
		if pathSegment == "" {
			pathSegment = toSnakeCase(schemaName) + "s"
		}

		discovered[schemaName] = resourceInfo{
			pathSegment: pathSegment,
			doc:         subDoc,
		}
	}

	return discovered, nil
}

func inferPathSegment(paths map[string]interface{}, apiPrefix, resourceName string) string {
	for pathKey := range paths {
		if !strings.HasPrefix(pathKey, apiPrefix+"/") {
			continue
		}

		remainder := strings.TrimPrefix(pathKey, apiPrefix+"/")
		segments := strings.Split(remainder, "/")
		if len(segments) >= 1 {
			segment := segments[0]
			segmentPascal := toPascalCase(segment)
			if segmentPascal == resourceName || segmentPascal == resourceName+"s" {
				return segment
			}
		}
	}
	return ""
}

func toPascalCase(s string) string {
	if len(s) == 0 {
		return s
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			if upper, ok := commonAcronyms[strings.ToUpper(part)]; ok {
				result.WriteString(upper)
			} else {
				result.WriteString(strings.ToUpper(string(part[0])) + part[1:])
			}
		}
	}
	return result.String()
}

func extractResource(name, pathSegment string, doc *subSpecDoc) (*Resource, error) {
	schema, ok := doc.Components.Schemas[name]
	if !ok {
		return nil, fmt.Errorf("schema %s not found", name)
	}

	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("schema %s is not a map", name)
	}

	fields, requiredFields, err := extractFields(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("extract fields for %s: %w", name, err)
	}

	patchName := name + "PatchRequest"
	patchSchema, ok := doc.Components.Schemas[patchName]
	var patchFields []Field
	if ok {
		patchMap, ok := patchSchema.(map[string]interface{})
		if ok {
			patchFields, _, err = extractPatchFields(patchMap)
			if err != nil {
				return nil, fmt.Errorf("extract patch fields for %s: %w", name, err)
			}
		}
	}

	statusPatchName := name + "StatusPatchRequest"
	statusPatchSchema, ok := doc.Components.Schemas[statusPatchName]
	var statusPatchFields []Field
	hasStatusPatch := false
	if ok {
		statusPatchMap, ok := statusPatchSchema.(map[string]interface{})
		if ok {
			statusPatchFields, _, err = extractPatchFields(statusPatchMap)
			if err != nil {
				return nil, fmt.Errorf("extract status patch fields for %s: %w", name, err)
			}
			hasStatusPatch = len(statusPatchFields) > 0
		}
	}

	hasDelete := checkHasOperation(doc.Paths, pathSegment, "delete")
	hasPatch := checkHasOperation(doc.Paths, pathSegment, "patch")
	actions := detectActions(doc.Paths, pathSegment)

	return &Resource{
		Name:              name,
		Plural:            resourcePlural(name),
		PathSegment:       pathSegment,
		Fields:            fields,
		RequiredFields:    requiredFields,
		PatchFields:       patchFields,
		StatusPatchFields: statusPatchFields,
		HasDelete:         hasDelete,
		HasPatch:          hasPatch,
		HasStatusPatch:    hasStatusPatch,
		Actions:           actions,
	}, nil
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

func extractFields(schemaMap map[string]interface{}) ([]Field, []string, error) {
	allOf, ok := schemaMap["allOf"]
	if !ok {
		return extractDirectFields(schemaMap)
	}

	allOfList, ok := allOf.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("allOf is not a list")
	}

	var fields []Field
	var requiredFields []string

	for _, item := range allOfList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if _, hasRef := itemMap["$ref"]; hasRef {
			continue
		}

		if req, ok := itemMap["required"]; ok {
			if reqList, ok := req.([]interface{}); ok {
				for _, r := range reqList {
					if s, ok := r.(string); ok {
						requiredFields = append(requiredFields, s)
					}
				}
			}
		}

		props, ok := itemMap["properties"]
		if !ok {
			continue
		}

		propsMap, ok := props.(map[string]interface{})
		if !ok {
			continue
		}

		for propName, propVal := range propsMap {
			if isObjectReferenceField(propName) {
				continue
			}

			propMap, ok := propVal.(map[string]interface{})
			if !ok {
				continue
			}

			propType, _ := propMap["type"].(string)
			propFormat, _ := propMap["format"].(string)
			readOnly, _ := propMap["readOnly"].(bool)

			isRequired := false
			for _, r := range requiredFields {
				if r == propName {
					isRequired = true
					break
				}
			}

			f := Field{
				Name:       propName,
				GoName:     toGoName(propName),
				PythonName: propName,
				TSName:     toCamelCase(propName),
				Type:       propType,
				Format:     propFormat,
				GoType:     toGoType(propType, propFormat),
				PythonType: toPythonType(propType, propFormat),
				TSType:     toTSType(propType, propFormat),
				Required:   isRequired,
				ReadOnly:   readOnly,
				JSONTag:    jsonTag(propName, isRequired),
			}

			fields = append(fields, f)
		}
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	return fields, requiredFields, nil
}

func extractDirectFields(schemaMap map[string]interface{}) ([]Field, []string, error) {
	props, ok := schemaMap["properties"]
	if !ok {
		return nil, nil, nil
	}

	propsMap, ok := props.(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	var requiredFields []string
	if req, ok := schemaMap["required"]; ok {
		if reqList, ok := req.([]interface{}); ok {
			for _, r := range reqList {
				if s, ok := r.(string); ok {
					requiredFields = append(requiredFields, s)
				}
			}
		}
	}

	var fields []Field
	for propName, propVal := range propsMap {
		if isObjectReferenceField(propName) {
			continue
		}

		propMap, ok := propVal.(map[string]interface{})
		if !ok {
			continue
		}

		propType, _ := propMap["type"].(string)
		propFormat, _ := propMap["format"].(string)
		readOnly, _ := propMap["readOnly"].(bool)

		isRequired := false
		for _, r := range requiredFields {
			if r == propName {
				isRequired = true
				break
			}
		}

		f := Field{
			Name:       propName,
			GoName:     toGoName(propName),
			PythonName: propName,
			TSName:     toCamelCase(propName),
			Type:       propType,
			Format:     propFormat,
			GoType:     toGoType(propType, propFormat),
			PythonType: toPythonType(propType, propFormat),
			TSType:     toTSType(propType, propFormat),
			Required:   isRequired,
			ReadOnly:   readOnly,
			JSONTag:    jsonTag(propName, isRequired),
		}

		fields = append(fields, f)
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	return fields, requiredFields, nil
}

func extractPatchFields(schemaMap map[string]interface{}) ([]Field, []string, error) {
	props, ok := schemaMap["properties"]
	if !ok {
		return nil, nil, nil
	}

	propsMap, ok := props.(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	var fields []Field
	for propName, propVal := range propsMap {
		propMap, ok := propVal.(map[string]interface{})
		if !ok {
			continue
		}

		propType, _ := propMap["type"].(string)
		propFormat, _ := propMap["format"].(string)

		f := Field{
			Name:       propName,
			GoName:     toGoName(propName),
			PythonName: propName,
			TSName:     toCamelCase(propName),
			Type:       propType,
			Format:     propFormat,
			GoType:     toGoType(propType, propFormat),
			PythonType: toPythonType(propType, propFormat),
			TSType:     toTSType(propType, propFormat),
			Required:   false,
			JSONTag:    jsonTag(propName, false),
		}

		fields = append(fields, f)
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	return fields, nil, nil
}

func checkHasOperation(paths map[string]interface{}, pathSegment, operation string) bool {
	for pathKey, pathVal := range paths {
		if !strings.Contains(pathKey, "/"+pathSegment+"/{id}") {
			continue
		}
		segments := strings.Split(pathKey, "/")
		if len(segments) > 0 && segments[len(segments)-1] != "{id}" {
			continue
		}

		pathMap, ok := pathVal.(map[string]interface{})
		if !ok {
			continue
		}

		if _, has := pathMap[operation]; has {
			return true
		}
	}
	return false
}

func detectActions(paths map[string]interface{}, pathSegment string) []string {
	var found []string
	for pathKey, pathVal := range paths {
		if !strings.Contains(pathKey, "/"+pathSegment+"/{id}/") {
			continue
		}

		segments := strings.Split(pathKey, "/")
		if len(segments) < 2 {
			continue
		}
		lastSegment := segments[len(segments)-1]
		if lastSegment == "{id}" || lastSegment == "status" {
			continue
		}

		pathMap, ok := pathVal.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasPost := pathMap["post"]; hasPost {
			found = append(found, lastSegment)
		}
	}
	sort.Strings(found)
	return found
}

var objectReferenceFields = map[string]bool{
	"id":         true,
	"kind":       true,
	"href":       true,
	"created_at": true,
	"updated_at": true,
}

func isObjectReferenceField(name string) bool {
	return objectReferenceFields[name]
}
