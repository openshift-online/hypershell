# AgentRuntimePatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**SandboxTemplateId** | Pointer to **string** |  | [optional] 
**Description** | Pointer to **string** |  | [optional] 
**Cron** | Pointer to **string** |  | [optional] 
**CoordinatorImage** | Pointer to **string** |  | [optional] 
**ConcurrencyPolicy** | Pointer to **string** |  | [optional] 
**LoginRefreshSeconds** | Pointer to **int32** |  | [optional] 
**Parameters** | Pointer to **string** |  | [optional] 

## Methods

### NewAgentRuntimePatchRequest

`func NewAgentRuntimePatchRequest() *AgentRuntimePatchRequest`

NewAgentRuntimePatchRequest instantiates a new AgentRuntimePatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAgentRuntimePatchRequestWithDefaults

`func NewAgentRuntimePatchRequestWithDefaults() *AgentRuntimePatchRequest`

NewAgentRuntimePatchRequestWithDefaults instantiates a new AgentRuntimePatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *AgentRuntimePatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *AgentRuntimePatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *AgentRuntimePatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *AgentRuntimePatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetSandboxTemplateId

`func (o *AgentRuntimePatchRequest) GetSandboxTemplateId() string`

GetSandboxTemplateId returns the SandboxTemplateId field if non-nil, zero value otherwise.

### GetSandboxTemplateIdOk

`func (o *AgentRuntimePatchRequest) GetSandboxTemplateIdOk() (*string, bool)`

GetSandboxTemplateIdOk returns a tuple with the SandboxTemplateId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSandboxTemplateId

`func (o *AgentRuntimePatchRequest) SetSandboxTemplateId(v string)`

SetSandboxTemplateId sets SandboxTemplateId field to given value.

### HasSandboxTemplateId

`func (o *AgentRuntimePatchRequest) HasSandboxTemplateId() bool`

HasSandboxTemplateId returns a boolean if a field has been set.

### GetDescription

`func (o *AgentRuntimePatchRequest) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *AgentRuntimePatchRequest) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *AgentRuntimePatchRequest) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *AgentRuntimePatchRequest) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### GetCron

`func (o *AgentRuntimePatchRequest) GetCron() string`

GetCron returns the Cron field if non-nil, zero value otherwise.

### GetCronOk

`func (o *AgentRuntimePatchRequest) GetCronOk() (*string, bool)`

GetCronOk returns a tuple with the Cron field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCron

`func (o *AgentRuntimePatchRequest) SetCron(v string)`

SetCron sets Cron field to given value.

### HasCron

`func (o *AgentRuntimePatchRequest) HasCron() bool`

HasCron returns a boolean if a field has been set.

### GetCoordinatorImage

`func (o *AgentRuntimePatchRequest) GetCoordinatorImage() string`

GetCoordinatorImage returns the CoordinatorImage field if non-nil, zero value otherwise.

### GetCoordinatorImageOk

`func (o *AgentRuntimePatchRequest) GetCoordinatorImageOk() (*string, bool)`

GetCoordinatorImageOk returns a tuple with the CoordinatorImage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCoordinatorImage

`func (o *AgentRuntimePatchRequest) SetCoordinatorImage(v string)`

SetCoordinatorImage sets CoordinatorImage field to given value.

### HasCoordinatorImage

`func (o *AgentRuntimePatchRequest) HasCoordinatorImage() bool`

HasCoordinatorImage returns a boolean if a field has been set.

### GetConcurrencyPolicy

`func (o *AgentRuntimePatchRequest) GetConcurrencyPolicy() string`

GetConcurrencyPolicy returns the ConcurrencyPolicy field if non-nil, zero value otherwise.

### GetConcurrencyPolicyOk

`func (o *AgentRuntimePatchRequest) GetConcurrencyPolicyOk() (*string, bool)`

GetConcurrencyPolicyOk returns a tuple with the ConcurrencyPolicy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConcurrencyPolicy

`func (o *AgentRuntimePatchRequest) SetConcurrencyPolicy(v string)`

SetConcurrencyPolicy sets ConcurrencyPolicy field to given value.

### HasConcurrencyPolicy

`func (o *AgentRuntimePatchRequest) HasConcurrencyPolicy() bool`

HasConcurrencyPolicy returns a boolean if a field has been set.

### GetLoginRefreshSeconds

`func (o *AgentRuntimePatchRequest) GetLoginRefreshSeconds() int32`

GetLoginRefreshSeconds returns the LoginRefreshSeconds field if non-nil, zero value otherwise.

### GetLoginRefreshSecondsOk

`func (o *AgentRuntimePatchRequest) GetLoginRefreshSecondsOk() (*int32, bool)`

GetLoginRefreshSecondsOk returns a tuple with the LoginRefreshSeconds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLoginRefreshSeconds

`func (o *AgentRuntimePatchRequest) SetLoginRefreshSeconds(v int32)`

SetLoginRefreshSeconds sets LoginRefreshSeconds field to given value.

### HasLoginRefreshSeconds

`func (o *AgentRuntimePatchRequest) HasLoginRefreshSeconds() bool`

HasLoginRefreshSeconds returns a boolean if a field has been set.

### GetParameters

`func (o *AgentRuntimePatchRequest) GetParameters() string`

GetParameters returns the Parameters field if non-nil, zero value otherwise.

### GetParametersOk

`func (o *AgentRuntimePatchRequest) GetParametersOk() (*string, bool)`

GetParametersOk returns a tuple with the Parameters field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetParameters

`func (o *AgentRuntimePatchRequest) SetParameters(v string)`

SetParameters sets Parameters field to given value.

### HasParameters

`func (o *AgentRuntimePatchRequest) HasParameters() bool`

HasParameters returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


