# ManagedClusterPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**Provider** | Pointer to **string** |  | [optional] 
**Region** | Pointer to **string** |  | [optional] 
**KubeconfigSecret** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 
**ApiServerUrl** | Pointer to **string** |  | [optional] 
**ProfileId** | Pointer to **string** | Default GatewayProfile for gateways on this cluster; set to empty string to clear | [optional] 
**DatabaseId** | Pointer to **string** |  | [optional] 

## Methods

### NewManagedClusterPatchRequest

`func NewManagedClusterPatchRequest() *ManagedClusterPatchRequest`

NewManagedClusterPatchRequest instantiates a new ManagedClusterPatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterPatchRequestWithDefaults

`func NewManagedClusterPatchRequestWithDefaults() *ManagedClusterPatchRequest`

NewManagedClusterPatchRequestWithDefaults instantiates a new ManagedClusterPatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *ManagedClusterPatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ManagedClusterPatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ManagedClusterPatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *ManagedClusterPatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetProvider

`func (o *ManagedClusterPatchRequest) GetProvider() string`

GetProvider returns the Provider field if non-nil, zero value otherwise.

### GetProviderOk

`func (o *ManagedClusterPatchRequest) GetProviderOk() (*string, bool)`

GetProviderOk returns a tuple with the Provider field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProvider

`func (o *ManagedClusterPatchRequest) SetProvider(v string)`

SetProvider sets Provider field to given value.

### HasProvider

`func (o *ManagedClusterPatchRequest) HasProvider() bool`

HasProvider returns a boolean if a field has been set.

### GetRegion

`func (o *ManagedClusterPatchRequest) GetRegion() string`

GetRegion returns the Region field if non-nil, zero value otherwise.

### GetRegionOk

`func (o *ManagedClusterPatchRequest) GetRegionOk() (*string, bool)`

GetRegionOk returns a tuple with the Region field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegion

`func (o *ManagedClusterPatchRequest) SetRegion(v string)`

SetRegion sets Region field to given value.

### HasRegion

`func (o *ManagedClusterPatchRequest) HasRegion() bool`

HasRegion returns a boolean if a field has been set.

### GetKubeconfigSecret

`func (o *ManagedClusterPatchRequest) GetKubeconfigSecret() string`

GetKubeconfigSecret returns the KubeconfigSecret field if non-nil, zero value otherwise.

### GetKubeconfigSecretOk

`func (o *ManagedClusterPatchRequest) GetKubeconfigSecretOk() (*string, bool)`

GetKubeconfigSecretOk returns a tuple with the KubeconfigSecret field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKubeconfigSecret

`func (o *ManagedClusterPatchRequest) SetKubeconfigSecret(v string)`

SetKubeconfigSecret sets KubeconfigSecret field to given value.

### HasKubeconfigSecret

`func (o *ManagedClusterPatchRequest) HasKubeconfigSecret() bool`

HasKubeconfigSecret returns a boolean if a field has been set.

### GetStatus

`func (o *ManagedClusterPatchRequest) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ManagedClusterPatchRequest) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ManagedClusterPatchRequest) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *ManagedClusterPatchRequest) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetApiServerUrl

`func (o *ManagedClusterPatchRequest) GetApiServerUrl() string`

GetApiServerUrl returns the ApiServerUrl field if non-nil, zero value otherwise.

### GetApiServerUrlOk

`func (o *ManagedClusterPatchRequest) GetApiServerUrlOk() (*string, bool)`

GetApiServerUrlOk returns a tuple with the ApiServerUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetApiServerUrl

`func (o *ManagedClusterPatchRequest) SetApiServerUrl(v string)`

SetApiServerUrl sets ApiServerUrl field to given value.

### HasApiServerUrl

`func (o *ManagedClusterPatchRequest) HasApiServerUrl() bool`

HasApiServerUrl returns a boolean if a field has been set.

### GetProfileId

`func (o *ManagedClusterPatchRequest) GetProfileId() string`

GetProfileId returns the ProfileId field if non-nil, zero value otherwise.

### GetProfileIdOk

`func (o *ManagedClusterPatchRequest) GetProfileIdOk() (*string, bool)`

GetProfileIdOk returns a tuple with the ProfileId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProfileId

`func (o *ManagedClusterPatchRequest) SetProfileId(v string)`

SetProfileId sets ProfileId field to given value.

### HasProfileId

`func (o *ManagedClusterPatchRequest) HasProfileId() bool`

HasProfileId returns a boolean if a field has been set.

### GetDatabaseId

`func (o *ManagedClusterPatchRequest) GetDatabaseId() string`

GetDatabaseId returns the DatabaseId field if non-nil, zero value otherwise.

### GetDatabaseIdOk

`func (o *ManagedClusterPatchRequest) GetDatabaseIdOk() (*string, bool)`

GetDatabaseIdOk returns a tuple with the DatabaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDatabaseId

`func (o *ManagedClusterPatchRequest) SetDatabaseId(v string)`

SetDatabaseId sets DatabaseId field to given value.

### HasDatabaseId

`func (o *ManagedClusterPatchRequest) HasDatabaseId() bool`

HasDatabaseId returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


