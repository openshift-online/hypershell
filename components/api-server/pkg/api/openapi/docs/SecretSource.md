# SecretSource

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**AgentRuntimeId** | **string** |  | 
**Purpose** | **string** | Semantic purpose driving controller-generated env var wiring | 
**Backend** | **string** | Secret backend type (vault, aws-secrets-manager, kubernetes) | 
**Path** | **string** | Path to the secret in the backend | 
**KeyMappings** | Pointer to **string** | JSON-encoded mapping of backend secret keys to environment variable names | [optional] 

## Methods

### NewSecretSource

`func NewSecretSource(name string, agentRuntimeId string, purpose string, backend string, path string, ) *SecretSource`

NewSecretSource instantiates a new SecretSource object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSecretSourceWithDefaults

`func NewSecretSourceWithDefaults() *SecretSource`

NewSecretSourceWithDefaults instantiates a new SecretSource object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *SecretSource) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *SecretSource) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *SecretSource) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *SecretSource) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *SecretSource) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *SecretSource) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *SecretSource) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *SecretSource) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *SecretSource) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *SecretSource) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *SecretSource) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *SecretSource) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *SecretSource) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *SecretSource) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *SecretSource) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *SecretSource) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *SecretSource) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *SecretSource) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *SecretSource) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *SecretSource) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *SecretSource) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *SecretSource) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *SecretSource) SetName(v string)`

SetName sets Name field to given value.


### GetAgentRuntimeId

`func (o *SecretSource) GetAgentRuntimeId() string`

GetAgentRuntimeId returns the AgentRuntimeId field if non-nil, zero value otherwise.

### GetAgentRuntimeIdOk

`func (o *SecretSource) GetAgentRuntimeIdOk() (*string, bool)`

GetAgentRuntimeIdOk returns a tuple with the AgentRuntimeId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAgentRuntimeId

`func (o *SecretSource) SetAgentRuntimeId(v string)`

SetAgentRuntimeId sets AgentRuntimeId field to given value.


### GetPurpose

`func (o *SecretSource) GetPurpose() string`

GetPurpose returns the Purpose field if non-nil, zero value otherwise.

### GetPurposeOk

`func (o *SecretSource) GetPurposeOk() (*string, bool)`

GetPurposeOk returns a tuple with the Purpose field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPurpose

`func (o *SecretSource) SetPurpose(v string)`

SetPurpose sets Purpose field to given value.


### GetBackend

`func (o *SecretSource) GetBackend() string`

GetBackend returns the Backend field if non-nil, zero value otherwise.

### GetBackendOk

`func (o *SecretSource) GetBackendOk() (*string, bool)`

GetBackendOk returns a tuple with the Backend field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBackend

`func (o *SecretSource) SetBackend(v string)`

SetBackend sets Backend field to given value.


### GetPath

`func (o *SecretSource) GetPath() string`

GetPath returns the Path field if non-nil, zero value otherwise.

### GetPathOk

`func (o *SecretSource) GetPathOk() (*string, bool)`

GetPathOk returns a tuple with the Path field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPath

`func (o *SecretSource) SetPath(v string)`

SetPath sets Path field to given value.


### GetKeyMappings

`func (o *SecretSource) GetKeyMappings() string`

GetKeyMappings returns the KeyMappings field if non-nil, zero value otherwise.

### GetKeyMappingsOk

`func (o *SecretSource) GetKeyMappingsOk() (*string, bool)`

GetKeyMappingsOk returns a tuple with the KeyMappings field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKeyMappings

`func (o *SecretSource) SetKeyMappings(v string)`

SetKeyMappings sets KeyMappings field to given value.

### HasKeyMappings

`func (o *SecretSource) HasKeyMappings() bool`

HasKeyMappings returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


