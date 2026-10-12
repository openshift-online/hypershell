# AgentRuntime

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**ClusterId** | **string** |  | 
**GatewayId** | **string** |  | 
**SandboxTemplateId** | Pointer to **string** |  | [optional] 
**RepositoryId** | Pointer to **string** | Optional reference to a Repository the runtime scans | [optional] 
**Selector** | Pointer to **[]string** | Labels selecting repository work items to scan (match-any); requires repository_id | [optional] 
**Description** | Pointer to **string** |  | [optional] 
**Cron** | Pointer to **string** | Cron expression for scheduling the agent coordinator | [optional] 
**CoordinatorImage** | Pointer to **string** | Container image for the agent coordinator CronJob | [optional] 
**ConcurrencyPolicy** | Pointer to **string** | Kubernetes CronJob concurrency policy (Allow, Forbid, Replace) | [optional] 
**LoginRefreshSeconds** | Pointer to **int32** | Interval in seconds for refreshing gateway login credentials | [optional] 
**Parameters** | Pointer to **string** | JSON-encoded key-value parameters passed to the agent coordinator | [optional] 
**Status** | Pointer to **string** |  | [optional] [readonly] 

## Methods

### NewAgentRuntime

`func NewAgentRuntime(name string, clusterId string, gatewayId string, ) *AgentRuntime`

NewAgentRuntime instantiates a new AgentRuntime object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAgentRuntimeWithDefaults

`func NewAgentRuntimeWithDefaults() *AgentRuntime`

NewAgentRuntimeWithDefaults instantiates a new AgentRuntime object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *AgentRuntime) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *AgentRuntime) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *AgentRuntime) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *AgentRuntime) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *AgentRuntime) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *AgentRuntime) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *AgentRuntime) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *AgentRuntime) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *AgentRuntime) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *AgentRuntime) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *AgentRuntime) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *AgentRuntime) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *AgentRuntime) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *AgentRuntime) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *AgentRuntime) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *AgentRuntime) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *AgentRuntime) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *AgentRuntime) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *AgentRuntime) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *AgentRuntime) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *AgentRuntime) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *AgentRuntime) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *AgentRuntime) SetName(v string)`

SetName sets Name field to given value.


### GetClusterId

`func (o *AgentRuntime) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *AgentRuntime) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *AgentRuntime) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.


### GetGatewayId

`func (o *AgentRuntime) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *AgentRuntime) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *AgentRuntime) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.


### GetSandboxTemplateId

`func (o *AgentRuntime) GetSandboxTemplateId() string`

GetSandboxTemplateId returns the SandboxTemplateId field if non-nil, zero value otherwise.

### GetSandboxTemplateIdOk

`func (o *AgentRuntime) GetSandboxTemplateIdOk() (*string, bool)`

GetSandboxTemplateIdOk returns a tuple with the SandboxTemplateId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSandboxTemplateId

`func (o *AgentRuntime) SetSandboxTemplateId(v string)`

SetSandboxTemplateId sets SandboxTemplateId field to given value.

### HasSandboxTemplateId

`func (o *AgentRuntime) HasSandboxTemplateId() bool`

HasSandboxTemplateId returns a boolean if a field has been set.

### GetRepositoryId

`func (o *AgentRuntime) GetRepositoryId() string`

GetRepositoryId returns the RepositoryId field if non-nil, zero value otherwise.

### GetRepositoryIdOk

`func (o *AgentRuntime) GetRepositoryIdOk() (*string, bool)`

GetRepositoryIdOk returns a tuple with the RepositoryId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRepositoryId

`func (o *AgentRuntime) SetRepositoryId(v string)`

SetRepositoryId sets RepositoryId field to given value.

### HasRepositoryId

`func (o *AgentRuntime) HasRepositoryId() bool`

HasRepositoryId returns a boolean if a field has been set.

### GetSelector

`func (o *AgentRuntime) GetSelector() []string`

GetSelector returns the Selector field if non-nil, zero value otherwise.

### GetSelectorOk

`func (o *AgentRuntime) GetSelectorOk() (*[]string, bool)`

GetSelectorOk returns a tuple with the Selector field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSelector

`func (o *AgentRuntime) SetSelector(v []string)`

SetSelector sets Selector field to given value.

### HasSelector

`func (o *AgentRuntime) HasSelector() bool`

HasSelector returns a boolean if a field has been set.

### GetDescription

`func (o *AgentRuntime) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *AgentRuntime) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *AgentRuntime) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *AgentRuntime) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### GetCron

`func (o *AgentRuntime) GetCron() string`

GetCron returns the Cron field if non-nil, zero value otherwise.

### GetCronOk

`func (o *AgentRuntime) GetCronOk() (*string, bool)`

GetCronOk returns a tuple with the Cron field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCron

`func (o *AgentRuntime) SetCron(v string)`

SetCron sets Cron field to given value.

### HasCron

`func (o *AgentRuntime) HasCron() bool`

HasCron returns a boolean if a field has been set.

### GetCoordinatorImage

`func (o *AgentRuntime) GetCoordinatorImage() string`

GetCoordinatorImage returns the CoordinatorImage field if non-nil, zero value otherwise.

### GetCoordinatorImageOk

`func (o *AgentRuntime) GetCoordinatorImageOk() (*string, bool)`

GetCoordinatorImageOk returns a tuple with the CoordinatorImage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCoordinatorImage

`func (o *AgentRuntime) SetCoordinatorImage(v string)`

SetCoordinatorImage sets CoordinatorImage field to given value.

### HasCoordinatorImage

`func (o *AgentRuntime) HasCoordinatorImage() bool`

HasCoordinatorImage returns a boolean if a field has been set.

### GetConcurrencyPolicy

`func (o *AgentRuntime) GetConcurrencyPolicy() string`

GetConcurrencyPolicy returns the ConcurrencyPolicy field if non-nil, zero value otherwise.

### GetConcurrencyPolicyOk

`func (o *AgentRuntime) GetConcurrencyPolicyOk() (*string, bool)`

GetConcurrencyPolicyOk returns a tuple with the ConcurrencyPolicy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConcurrencyPolicy

`func (o *AgentRuntime) SetConcurrencyPolicy(v string)`

SetConcurrencyPolicy sets ConcurrencyPolicy field to given value.

### HasConcurrencyPolicy

`func (o *AgentRuntime) HasConcurrencyPolicy() bool`

HasConcurrencyPolicy returns a boolean if a field has been set.

### GetLoginRefreshSeconds

`func (o *AgentRuntime) GetLoginRefreshSeconds() int32`

GetLoginRefreshSeconds returns the LoginRefreshSeconds field if non-nil, zero value otherwise.

### GetLoginRefreshSecondsOk

`func (o *AgentRuntime) GetLoginRefreshSecondsOk() (*int32, bool)`

GetLoginRefreshSecondsOk returns a tuple with the LoginRefreshSeconds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLoginRefreshSeconds

`func (o *AgentRuntime) SetLoginRefreshSeconds(v int32)`

SetLoginRefreshSeconds sets LoginRefreshSeconds field to given value.

### HasLoginRefreshSeconds

`func (o *AgentRuntime) HasLoginRefreshSeconds() bool`

HasLoginRefreshSeconds returns a boolean if a field has been set.

### GetParameters

`func (o *AgentRuntime) GetParameters() string`

GetParameters returns the Parameters field if non-nil, zero value otherwise.

### GetParametersOk

`func (o *AgentRuntime) GetParametersOk() (*string, bool)`

GetParametersOk returns a tuple with the Parameters field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetParameters

`func (o *AgentRuntime) SetParameters(v string)`

SetParameters sets Parameters field to given value.

### HasParameters

`func (o *AgentRuntime) HasParameters() bool`

HasParameters returns a boolean if a field has been set.

### GetStatus

`func (o *AgentRuntime) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *AgentRuntime) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *AgentRuntime) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *AgentRuntime) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


