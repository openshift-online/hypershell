# EffectivePolicy

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional]
**Name** | Pointer to **string** |  | [optional]
**UserMessage** | Pointer to **string** |  | [optional]
**DocumentationUrl** | Pointer to **string** |  | [optional]
**EnforcementMode** | Pointer to **string** |  | [optional]
**EffectiveMode** | Pointer to **string** |  | [optional]
**EffectiveModeReason** | Pointer to **string** | Why the policy is enforced or audit-only (policy setting or platform enforcement) | [optional]
**BuiltIn** | Pointer to **bool** |  | [optional]
**Cedar** | Pointer to **string** |  | [optional]
**Rendered** | Pointer to [**[]PolicyRenderedRule**](PolicyRenderedRule.md) |  | [optional]
**Condition** | Pointer to **string** | The rollout condition on this gateway (Applied, Pending, Unsupported) | [optional]
**Revision** | Pointer to **string** |  | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**Target** | Pointer to [**PlatformGatewayPolicyTarget**](PlatformGatewayPolicyTarget.md) |  | [optional]

## Methods

### NewEffectivePolicy

`func NewEffectivePolicy() *EffectivePolicy`

NewEffectivePolicy instantiates a new EffectivePolicy object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewEffectivePolicyWithDefaults

`func NewEffectivePolicyWithDefaults() *EffectivePolicy`

NewEffectivePolicyWithDefaults instantiates a new EffectivePolicy object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *EffectivePolicy) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *EffectivePolicy) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *EffectivePolicy) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *EffectivePolicy) HasId() bool`

HasId returns a boolean if a field has been set.

### GetName

`func (o *EffectivePolicy) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *EffectivePolicy) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *EffectivePolicy) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *EffectivePolicy) HasName() bool`

HasName returns a boolean if a field has been set.

### GetUserMessage

`func (o *EffectivePolicy) GetUserMessage() string`

GetUserMessage returns the UserMessage field if non-nil, zero value otherwise.

### GetUserMessageOk

`func (o *EffectivePolicy) GetUserMessageOk() (*string, bool)`

GetUserMessageOk returns a tuple with the UserMessage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUserMessage

`func (o *EffectivePolicy) SetUserMessage(v string)`

SetUserMessage sets UserMessage field to given value.

### HasUserMessage

`func (o *EffectivePolicy) HasUserMessage() bool`

HasUserMessage returns a boolean if a field has been set.

### GetDocumentationUrl

`func (o *EffectivePolicy) GetDocumentationUrl() string`

GetDocumentationUrl returns the DocumentationUrl field if non-nil, zero value otherwise.

### GetDocumentationUrlOk

`func (o *EffectivePolicy) GetDocumentationUrlOk() (*string, bool)`

GetDocumentationUrlOk returns a tuple with the DocumentationUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDocumentationUrl

`func (o *EffectivePolicy) SetDocumentationUrl(v string)`

SetDocumentationUrl sets DocumentationUrl field to given value.

### HasDocumentationUrl

`func (o *EffectivePolicy) HasDocumentationUrl() bool`

HasDocumentationUrl returns a boolean if a field has been set.

### GetEnforcementMode

`func (o *EffectivePolicy) GetEnforcementMode() string`

GetEnforcementMode returns the EnforcementMode field if non-nil, zero value otherwise.

### GetEnforcementModeOk

`func (o *EffectivePolicy) GetEnforcementModeOk() (*string, bool)`

GetEnforcementModeOk returns a tuple with the EnforcementMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEnforcementMode

`func (o *EffectivePolicy) SetEnforcementMode(v string)`

SetEnforcementMode sets EnforcementMode field to given value.

### HasEnforcementMode

`func (o *EffectivePolicy) HasEnforcementMode() bool`

HasEnforcementMode returns a boolean if a field has been set.

### GetEffectiveMode

`func (o *EffectivePolicy) GetEffectiveMode() string`

GetEffectiveMode returns the EffectiveMode field if non-nil, zero value otherwise.

### GetEffectiveModeOk

`func (o *EffectivePolicy) GetEffectiveModeOk() (*string, bool)`

GetEffectiveModeOk returns a tuple with the EffectiveMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEffectiveMode

`func (o *EffectivePolicy) SetEffectiveMode(v string)`

SetEffectiveMode sets EffectiveMode field to given value.

### HasEffectiveMode

`func (o *EffectivePolicy) HasEffectiveMode() bool`

HasEffectiveMode returns a boolean if a field has been set.

### GetEffectiveModeReason

`func (o *EffectivePolicy) GetEffectiveModeReason() string`

GetEffectiveModeReason returns the EffectiveModeReason field if non-nil, zero value otherwise.

### GetEffectiveModeReasonOk

`func (o *EffectivePolicy) GetEffectiveModeReasonOk() (*string, bool)`

GetEffectiveModeReasonOk returns a tuple with the EffectiveModeReason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEffectiveModeReason

`func (o *EffectivePolicy) SetEffectiveModeReason(v string)`

SetEffectiveModeReason sets EffectiveModeReason field to given value.

### HasEffectiveModeReason

`func (o *EffectivePolicy) HasEffectiveModeReason() bool`

HasEffectiveModeReason returns a boolean if a field has been set.

### GetBuiltIn

`func (o *EffectivePolicy) GetBuiltIn() bool`

GetBuiltIn returns the BuiltIn field if non-nil, zero value otherwise.

### GetBuiltInOk

`func (o *EffectivePolicy) GetBuiltInOk() (*bool, bool)`

GetBuiltInOk returns a tuple with the BuiltIn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBuiltIn

`func (o *EffectivePolicy) SetBuiltIn(v bool)`

SetBuiltIn sets BuiltIn field to given value.

### HasBuiltIn

`func (o *EffectivePolicy) HasBuiltIn() bool`

HasBuiltIn returns a boolean if a field has been set.

### GetCedar

`func (o *EffectivePolicy) GetCedar() string`

GetCedar returns the Cedar field if non-nil, zero value otherwise.

### GetCedarOk

`func (o *EffectivePolicy) GetCedarOk() (*string, bool)`

GetCedarOk returns a tuple with the Cedar field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCedar

`func (o *EffectivePolicy) SetCedar(v string)`

SetCedar sets Cedar field to given value.

### HasCedar

`func (o *EffectivePolicy) HasCedar() bool`

HasCedar returns a boolean if a field has been set.

### GetRendered

`func (o *EffectivePolicy) GetRendered() []PolicyRenderedRule`

GetRendered returns the Rendered field if non-nil, zero value otherwise.

### GetRenderedOk

`func (o *EffectivePolicy) GetRenderedOk() (*[]PolicyRenderedRule, bool)`

GetRenderedOk returns a tuple with the Rendered field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRendered

`func (o *EffectivePolicy) SetRendered(v []PolicyRenderedRule)`

SetRendered sets Rendered field to given value.

### HasRendered

`func (o *EffectivePolicy) HasRendered() bool`

HasRendered returns a boolean if a field has been set.

### GetCondition

`func (o *EffectivePolicy) GetCondition() string`

GetCondition returns the Condition field if non-nil, zero value otherwise.

### GetConditionOk

`func (o *EffectivePolicy) GetConditionOk() (*string, bool)`

GetConditionOk returns a tuple with the Condition field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCondition

`func (o *EffectivePolicy) SetCondition(v string)`

SetCondition sets Condition field to given value.

### HasCondition

`func (o *EffectivePolicy) HasCondition() bool`

HasCondition returns a boolean if a field has been set.

### GetRevision

`func (o *EffectivePolicy) GetRevision() string`

GetRevision returns the Revision field if non-nil, zero value otherwise.

### GetRevisionOk

`func (o *EffectivePolicy) GetRevisionOk() (*string, bool)`

GetRevisionOk returns a tuple with the Revision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevision

`func (o *EffectivePolicy) SetRevision(v string)`

SetRevision sets Revision field to given value.

### HasRevision

`func (o *EffectivePolicy) HasRevision() bool`

HasRevision returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *EffectivePolicy) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *EffectivePolicy) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *EffectivePolicy) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *EffectivePolicy) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetTarget

`func (o *EffectivePolicy) GetTarget() PlatformGatewayPolicyTarget`

GetTarget returns the Target field if non-nil, zero value otherwise.

### GetTargetOk

`func (o *EffectivePolicy) GetTargetOk() (*PlatformGatewayPolicyTarget, bool)`

GetTargetOk returns a tuple with the Target field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTarget

`func (o *EffectivePolicy) SetTarget(v PlatformGatewayPolicyTarget)`

SetTarget sets Target field to given value.

### HasTarget

`func (o *EffectivePolicy) HasTarget() bool`

HasTarget returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
