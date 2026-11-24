# PlatformGatewayPolicy

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional]
**Kind** | Pointer to **string** |  | [optional]
**Href** | Pointer to **string** |  | [optional]
**CreatedAt** | Pointer to **time.Time** |  | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**Name** | **string** | Stable identifier shown to users and used as the Cedar @id attribution (PLAT-PGP-14) |
**Description** | Pointer to **string** |  | [optional]
**Target** | Pointer to [**PlatformGatewayPolicyTarget**](PlatformGatewayPolicyTarget.md) |  | [optional]
**EnforcementMode** | Pointer to **string** | audit or enforce. A new policy is always stored as audit (PLAT-PGP-01). | [optional]
**EffectiveMode** | Pointer to **string** | The stricter of enforcement_mode and the global toggle (PLAT-PGP-06) | [optional] [readonly]
**UserMessage** | Pointer to **string** | Text shown in the denial reason and the UI. Must not contain secrets. | [optional]
**DocumentationUrl** | Pointer to **string** |  | [optional]
**Cedar** | **string** | One or more Cedar forbid statements (PLAT-PGP-04) |
**BuiltIn** | Pointer to **bool** |  | [optional] [readonly]
**Rendered** | Pointer to [**[]PolicyRenderedRule**](PolicyRenderedRule.md) |  | [optional] [readonly]
**Status** | Pointer to [**PlatformGatewayPolicyStatus**](PlatformGatewayPolicyStatus.md) |  | [optional]

## Methods

### NewPlatformGatewayPolicy

`func NewPlatformGatewayPolicy(name string, cedar string, ) *PlatformGatewayPolicy`

NewPlatformGatewayPolicy instantiates a new PlatformGatewayPolicy object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyWithDefaults

`func NewPlatformGatewayPolicyWithDefaults() *PlatformGatewayPolicy`

NewPlatformGatewayPolicyWithDefaults instantiates a new PlatformGatewayPolicy object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *PlatformGatewayPolicy) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *PlatformGatewayPolicy) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *PlatformGatewayPolicy) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *PlatformGatewayPolicy) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *PlatformGatewayPolicy) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *PlatformGatewayPolicy) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *PlatformGatewayPolicy) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *PlatformGatewayPolicy) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *PlatformGatewayPolicy) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *PlatformGatewayPolicy) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *PlatformGatewayPolicy) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *PlatformGatewayPolicy) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *PlatformGatewayPolicy) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *PlatformGatewayPolicy) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *PlatformGatewayPolicy) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *PlatformGatewayPolicy) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *PlatformGatewayPolicy) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *PlatformGatewayPolicy) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *PlatformGatewayPolicy) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *PlatformGatewayPolicy) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *PlatformGatewayPolicy) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *PlatformGatewayPolicy) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *PlatformGatewayPolicy) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *PlatformGatewayPolicy) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *PlatformGatewayPolicy) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *PlatformGatewayPolicy) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *PlatformGatewayPolicy) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### GetTarget

`func (o *PlatformGatewayPolicy) GetTarget() PlatformGatewayPolicyTarget`

GetTarget returns the Target field if non-nil, zero value otherwise.

### GetTargetOk

`func (o *PlatformGatewayPolicy) GetTargetOk() (*PlatformGatewayPolicyTarget, bool)`

GetTargetOk returns a tuple with the Target field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTarget

`func (o *PlatformGatewayPolicy) SetTarget(v PlatformGatewayPolicyTarget)`

SetTarget sets Target field to given value.

### HasTarget

`func (o *PlatformGatewayPolicy) HasTarget() bool`

HasTarget returns a boolean if a field has been set.

### GetEnforcementMode

`func (o *PlatformGatewayPolicy) GetEnforcementMode() string`

GetEnforcementMode returns the EnforcementMode field if non-nil, zero value otherwise.

### GetEnforcementModeOk

`func (o *PlatformGatewayPolicy) GetEnforcementModeOk() (*string, bool)`

GetEnforcementModeOk returns a tuple with the EnforcementMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEnforcementMode

`func (o *PlatformGatewayPolicy) SetEnforcementMode(v string)`

SetEnforcementMode sets EnforcementMode field to given value.

### HasEnforcementMode

`func (o *PlatformGatewayPolicy) HasEnforcementMode() bool`

HasEnforcementMode returns a boolean if a field has been set.

### GetEffectiveMode

`func (o *PlatformGatewayPolicy) GetEffectiveMode() string`

GetEffectiveMode returns the EffectiveMode field if non-nil, zero value otherwise.

### GetEffectiveModeOk

`func (o *PlatformGatewayPolicy) GetEffectiveModeOk() (*string, bool)`

GetEffectiveModeOk returns a tuple with the EffectiveMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEffectiveMode

`func (o *PlatformGatewayPolicy) SetEffectiveMode(v string)`

SetEffectiveMode sets EffectiveMode field to given value.

### HasEffectiveMode

`func (o *PlatformGatewayPolicy) HasEffectiveMode() bool`

HasEffectiveMode returns a boolean if a field has been set.

### GetUserMessage

`func (o *PlatformGatewayPolicy) GetUserMessage() string`

GetUserMessage returns the UserMessage field if non-nil, zero value otherwise.

### GetUserMessageOk

`func (o *PlatformGatewayPolicy) GetUserMessageOk() (*string, bool)`

GetUserMessageOk returns a tuple with the UserMessage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUserMessage

`func (o *PlatformGatewayPolicy) SetUserMessage(v string)`

SetUserMessage sets UserMessage field to given value.

### HasUserMessage

`func (o *PlatformGatewayPolicy) HasUserMessage() bool`

HasUserMessage returns a boolean if a field has been set.

### GetDocumentationUrl

`func (o *PlatformGatewayPolicy) GetDocumentationUrl() string`

GetDocumentationUrl returns the DocumentationUrl field if non-nil, zero value otherwise.

### GetDocumentationUrlOk

`func (o *PlatformGatewayPolicy) GetDocumentationUrlOk() (*string, bool)`

GetDocumentationUrlOk returns a tuple with the DocumentationUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDocumentationUrl

`func (o *PlatformGatewayPolicy) SetDocumentationUrl(v string)`

SetDocumentationUrl sets DocumentationUrl field to given value.

### HasDocumentationUrl

`func (o *PlatformGatewayPolicy) HasDocumentationUrl() bool`

HasDocumentationUrl returns a boolean if a field has been set.

### GetCedar

`func (o *PlatformGatewayPolicy) GetCedar() string`

GetCedar returns the Cedar field if non-nil, zero value otherwise.

### GetCedarOk

`func (o *PlatformGatewayPolicy) GetCedarOk() (*string, bool)`

GetCedarOk returns a tuple with the Cedar field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCedar

`func (o *PlatformGatewayPolicy) SetCedar(v string)`

SetCedar sets Cedar field to given value.


### GetBuiltIn

`func (o *PlatformGatewayPolicy) GetBuiltIn() bool`

GetBuiltIn returns the BuiltIn field if non-nil, zero value otherwise.

### GetBuiltInOk

`func (o *PlatformGatewayPolicy) GetBuiltInOk() (*bool, bool)`

GetBuiltInOk returns a tuple with the BuiltIn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBuiltIn

`func (o *PlatformGatewayPolicy) SetBuiltIn(v bool)`

SetBuiltIn sets BuiltIn field to given value.

### HasBuiltIn

`func (o *PlatformGatewayPolicy) HasBuiltIn() bool`

HasBuiltIn returns a boolean if a field has been set.

### GetRendered

`func (o *PlatformGatewayPolicy) GetRendered() []PolicyRenderedRule`

GetRendered returns the Rendered field if non-nil, zero value otherwise.

### GetRenderedOk

`func (o *PlatformGatewayPolicy) GetRenderedOk() (*[]PolicyRenderedRule, bool)`

GetRenderedOk returns a tuple with the Rendered field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRendered

`func (o *PlatformGatewayPolicy) SetRendered(v []PolicyRenderedRule)`

SetRendered sets Rendered field to given value.

### HasRendered

`func (o *PlatformGatewayPolicy) HasRendered() bool`

HasRendered returns a boolean if a field has been set.

### GetStatus

`func (o *PlatformGatewayPolicy) GetStatus() PlatformGatewayPolicyStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *PlatformGatewayPolicy) GetStatusOk() (*PlatformGatewayPolicyStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *PlatformGatewayPolicy) SetStatus(v PlatformGatewayPolicyStatus)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *PlatformGatewayPolicy) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
