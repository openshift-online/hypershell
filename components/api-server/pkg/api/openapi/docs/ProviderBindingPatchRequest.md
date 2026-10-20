# ProviderBindingPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**SecretSourceId** | Pointer to **string** |  | [optional] 
**RefreshStrategy** | Pointer to **string** |  | [optional] 

## Methods

### NewProviderBindingPatchRequest

`func NewProviderBindingPatchRequest() *ProviderBindingPatchRequest`

NewProviderBindingPatchRequest instantiates a new ProviderBindingPatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewProviderBindingPatchRequestWithDefaults

`func NewProviderBindingPatchRequestWithDefaults() *ProviderBindingPatchRequest`

NewProviderBindingPatchRequestWithDefaults instantiates a new ProviderBindingPatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *ProviderBindingPatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ProviderBindingPatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ProviderBindingPatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *ProviderBindingPatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetSecretSourceId

`func (o *ProviderBindingPatchRequest) GetSecretSourceId() string`

GetSecretSourceId returns the SecretSourceId field if non-nil, zero value otherwise.

### GetSecretSourceIdOk

`func (o *ProviderBindingPatchRequest) GetSecretSourceIdOk() (*string, bool)`

GetSecretSourceIdOk returns a tuple with the SecretSourceId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSecretSourceId

`func (o *ProviderBindingPatchRequest) SetSecretSourceId(v string)`

SetSecretSourceId sets SecretSourceId field to given value.

### HasSecretSourceId

`func (o *ProviderBindingPatchRequest) HasSecretSourceId() bool`

HasSecretSourceId returns a boolean if a field has been set.

### GetRefreshStrategy

`func (o *ProviderBindingPatchRequest) GetRefreshStrategy() string`

GetRefreshStrategy returns the RefreshStrategy field if non-nil, zero value otherwise.

### GetRefreshStrategyOk

`func (o *ProviderBindingPatchRequest) GetRefreshStrategyOk() (*string, bool)`

GetRefreshStrategyOk returns a tuple with the RefreshStrategy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRefreshStrategy

`func (o *ProviderBindingPatchRequest) SetRefreshStrategy(v string)`

SetRefreshStrategy sets RefreshStrategy field to given value.

### HasRefreshStrategy

`func (o *ProviderBindingPatchRequest) HasRefreshStrategy() bool`

HasRefreshStrategy returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


