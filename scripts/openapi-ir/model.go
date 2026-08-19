// Package ir loads OpenAPI documents into the canonical, operation-oriented
// intermediate representation shared by TRex generators.
package ir

import "sort"

type SourceLocation struct {
	File    string `json:"file"`
	Pointer string `json:"pointer"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

type ExtensionValue struct {
	Value  any            `json:"value"`
	Source SourceLocation `json:"source"`
}

type Document struct {
	OpenAPI         string                    `json:"openapi"`
	Title           string                    `json:"title"`
	Version         string                    `json:"version"`
	Source          SourceLocation            `json:"source"`
	Servers         []*Server                 `json:"servers,omitempty"`
	Security        []SecurityRequirement     `json:"security,omitempty"`
	SecuritySchemes []*SecurityScheme         `json:"securitySchemes,omitempty"`
	Extensions      map[string]ExtensionValue `json:"extensions,omitempty"`
	Operations      []*Operation              `json:"operations"`
	Schemas         []*Schema                 `json:"schemas"`
	SchemaUses      []*SchemaUse              `json:"schemaUses,omitempty"`
	ResourceViews   []*ResourceView           `json:"resourceViews,omitempty"`
	Relationships   []*Relationship           `json:"relationships,omitempty"`
}

type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
	Extensions  map[string]ExtensionValue `json:"extensions,omitempty"`
	Source      SourceLocation            `json:"source"`
}

type ServerVariable struct {
	Default     string   `json:"default"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

type SecurityScheme struct {
	Name             string                    `json:"name"`
	Type             string                    `json:"type"`
	Description      string                    `json:"description,omitempty"`
	ParameterName    string                    `json:"parameterName,omitempty"`
	In               string                    `json:"in,omitempty"`
	Scheme           string                    `json:"scheme,omitempty"`
	BearerFormat     string                    `json:"bearerFormat,omitempty"`
	OpenIDConnectURL string                    `json:"openIdConnectUrl,omitempty"`
	Extensions       map[string]ExtensionValue `json:"extensions,omitempty"`
	Source           SourceLocation            `json:"source"`
}

type SecurityState string

const (
	SecurityInherited SecurityState = "inherited"
	SecurityNone      SecurityState = "none"
	SecurityOverride  SecurityState = "override"
)

type OperationSecurity struct {
	State        SecurityState         `json:"state"`
	Requirements []SecurityRequirement `json:"requirements,omitempty"`
}

type SecurityRequirement struct {
	Schemes []SecuritySchemeUse `json:"schemes"`
}

type SecuritySchemeUse struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes,omitempty"`
}

type Operation struct {
	ID             string                    `json:"id"`
	Method         string                    `json:"method"`
	Path           string                    `json:"path"`
	Segments       []*RouteSegment           `json:"segments"`
	Parameters     []*Parameter              `json:"parameters,omitempty"`
	PathParameters []*Parameter              `json:"pathParameters,omitempty"`
	RequestBody    *RequestBody              `json:"requestBody,omitempty"`
	Responses      []*Response               `json:"responses"`
	Servers        []*Server                 `json:"servers,omitempty"`
	Security       OperationSecurity         `json:"security"`
	Tags           []string                  `json:"tags,omitempty"`
	Deprecated     bool                      `json:"deprecated,omitempty"`
	Summary        string                    `json:"summary,omitempty"`
	Description    string                    `json:"description,omitempty"`
	Capabilities   Capabilities              `json:"capabilities,omitempty"`
	Extensions     map[string]ExtensionValue `json:"extensions,omitempty"`
	PathExtensions map[string]ExtensionValue `json:"pathExtensions,omitempty"`
	Source         SourceLocation            `json:"source"`
}

type RouteSegment struct {
	Literal   string     `json:"literal,omitempty"`
	Parameter *Parameter `json:"parameter,omitempty"`
}

type Parameter struct {
	Name          string                    `json:"name"`
	In            string                    `json:"in"`
	Description   string                    `json:"description,omitempty"`
	Required      bool                      `json:"required"`
	Deprecated    bool                      `json:"deprecated,omitempty"`
	Style         string                    `json:"style"`
	Explode       bool                      `json:"explode"`
	AllowReserved bool                      `json:"allowReserved,omitempty"`
	Schema        *SchemaReference          `json:"schema,omitempty"`
	Example       any                       `json:"example,omitempty"`
	Extensions    map[string]ExtensionValue `json:"extensions,omitempty"`
	Source        SourceLocation            `json:"source"`
}

type RequestBody struct {
	Description string                    `json:"description,omitempty"`
	Required    bool                      `json:"required"`
	Content     []*MediaType              `json:"content"`
	Extensions  map[string]ExtensionValue `json:"extensions,omitempty"`
	Source      SourceLocation            `json:"source"`
}

type Response struct {
	Status      string                    `json:"status"`
	Description string                    `json:"description,omitempty"`
	Content     []*MediaType              `json:"content,omitempty"`
	Links       []*OperationLink          `json:"links,omitempty"`
	Extensions  map[string]ExtensionValue `json:"extensions,omitempty"`
	Source      SourceLocation            `json:"source"`
}

type MediaType struct {
	ContentType string           `json:"contentType"`
	Schema      *SchemaReference `json:"schema,omitempty"`
	Example     any              `json:"example,omitempty"`
}

type OperationLink struct {
	Name               string                    `json:"name"`
	TargetOperationID  string                    `json:"targetOperationId,omitempty"`
	TargetOperationRef string                    `json:"targetOperationRef,omitempty"`
	Parameters         []ParameterMapping        `json:"parameters,omitempty"`
	Description        string                    `json:"description,omitempty"`
	Extensions         map[string]ExtensionValue `json:"extensions,omitempty"`
	Source             SourceLocation            `json:"source"`
}

type ParameterMapping struct {
	Target     string `json:"target"`
	Expression any    `json:"expression"`
}

type SchemaReference struct {
	Ref string `json:"ref"`
}

type Schema struct {
	Ref                  string                    `json:"ref"`
	Name                 string                    `json:"name,omitempty"`
	Types                []string                  `json:"types,omitempty"`
	Title                string                    `json:"title,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	Properties           map[string]*Property      `json:"properties,omitempty"`
	Items                *SchemaReference          `json:"items,omitempty"`
	AdditionalProperties *SchemaReference          `json:"additionalProperties,omitempty"`
	AdditionalAllowed    *bool                     `json:"additionalAllowed,omitempty"`
	AllOf                []*SchemaReference        `json:"allOf,omitempty"`
	OneOf                []*SchemaReference        `json:"oneOf,omitempty"`
	AnyOf                []*SchemaReference        `json:"anyOf,omitempty"`
	Not                  *SchemaReference          `json:"not,omitempty"`
	Nullable             bool                      `json:"nullable,omitempty"`
	ReadOnly             bool                      `json:"readOnly,omitempty"`
	WriteOnly            bool                      `json:"writeOnly,omitempty"`
	Deprecated           bool                      `json:"deprecated,omitempty"`
	Enum                 []any                     `json:"enum,omitempty"`
	Default              any                       `json:"default,omitempty"`
	Example              any                       `json:"example,omitempty"`
	Minimum              *float64                  `json:"minimum,omitempty"`
	Maximum              *float64                  `json:"maximum,omitempty"`
	ExclusiveMinimum     bool                      `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     bool                      `json:"exclusiveMaximum,omitempty"`
	MultipleOf           *float64                  `json:"multipleOf,omitempty"`
	MinLength            uint64                    `json:"minLength,omitempty"`
	MaxLength            *uint64                   `json:"maxLength,omitempty"`
	Pattern              string                    `json:"pattern,omitempty"`
	MinItems             uint64                    `json:"minItems,omitempty"`
	MaxItems             *uint64                   `json:"maxItems,omitempty"`
	UniqueItems          bool                      `json:"uniqueItems,omitempty"`
	MinProperties        uint64                    `json:"minProperties,omitempty"`
	MaxProperties        *uint64                   `json:"maxProperties,omitempty"`
	Discriminator        *Discriminator            `json:"discriminator,omitempty"`
	Extensions           map[string]ExtensionValue `json:"extensions,omitempty"`
	Source               SourceLocation            `json:"source"`
}

type Property struct {
	Name        string                    `json:"name"`
	Schema      *SchemaReference          `json:"schema"`
	Required    bool                      `json:"required"`
	ReadOnly    bool                      `json:"readOnly,omitempty"`
	WriteOnly   bool                      `json:"writeOnly,omitempty"`
	Nullable    bool                      `json:"nullable,omitempty"`
	Description string                    `json:"description,omitempty"`
	Extensions  map[string]ExtensionValue `json:"extensions,omitempty"`
	Source      SourceLocation            `json:"source"`
}

type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

type SchemaRole string

const (
	SchemaRoleRequest   SchemaRole = "request"
	SchemaRoleResponse  SchemaRole = "response"
	SchemaRoleListItem  SchemaRole = "list-item"
	SchemaRoleError     SchemaRole = "error"
	SchemaRoleParameter SchemaRole = "parameter"
	SchemaRoleEvent     SchemaRole = "event"
)

type SchemaUse struct {
	OperationID string     `json:"operationId"`
	SchemaRef   string     `json:"schemaRef"`
	Role        SchemaRole `json:"role"`
	Context     string     `json:"context,omitempty"`
}

type ResourceViewKind string

const (
	ResourceCollection ResourceViewKind = "collection"
	ResourceItem       ResourceViewKind = "item"
)

type ResourceView struct {
	ID              string                    `json:"id"`
	Kind            ResourceViewKind          `json:"kind"`
	Path            string                    `json:"path"`
	SchemaRef       string                    `json:"schemaRef"`
	ScopeParameters []string                  `json:"scopeParameters,omitempty"`
	OperationIDs    []string                  `json:"operationIds"`
	Capabilities    Capabilities              `json:"capabilities,omitempty"`
	Extensions      map[string]ExtensionValue `json:"extensions,omitempty"`
}

type RelationshipProvenance string

const (
	RelationshipExplicit RelationshipProvenance = "explicit-link"
	RelationshipInferred RelationshipProvenance = "inferred-path"
)

type Relationship struct {
	Name              string                 `json:"name"`
	SourceOperationID string                 `json:"sourceOperationId,omitempty"`
	TargetOperationID string                 `json:"targetOperationId,omitempty"`
	SourceViewID      string                 `json:"sourceViewId,omitempty"`
	TargetViewID      string                 `json:"targetViewId,omitempty"`
	ParameterMappings []ParameterMapping     `json:"parameterMappings,omitempty"`
	Provenance        RelationshipProvenance `json:"provenance"`
	Source            SourceLocation         `json:"source"`
}

type Capability string

const (
	CapabilityList   Capability = "list"
	CapabilityGet    Capability = "get"
	CapabilityCreate Capability = "create"
	CapabilityUpdate Capability = "update"
	CapabilityDelete Capability = "delete"
	CapabilityAction Capability = "action"
	CapabilityStream Capability = "stream"
)

type Capabilities []Capability

func (capabilities Capabilities) Has(want Capability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func (document *Document) Operation(id string) *Operation {
	for _, operation := range document.Operations {
		if operation.ID == id {
			return operation
		}
	}
	return nil
}

func (document *Document) Schema(ref string) *Schema {
	for _, schema := range document.Schemas {
		if schema.Ref == ref || schema.Name == ref {
			return schema
		}
	}
	return nil
}

func sortCapabilities(capabilities Capabilities) Capabilities {
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	result := capabilities[:0]
	for _, capability := range capabilities {
		if len(result) == 0 || result[len(result)-1] != capability {
			result = append(result, capability)
		}
	}
	return result
}
