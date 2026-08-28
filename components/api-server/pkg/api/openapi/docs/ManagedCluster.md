# ManagedCluster

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Provider** | **string** |  | 
**Region** | Pointer to **string** |  | [optional] 
**KubeconfigSecret** | **string** |  | 
**Status** | Pointer to **string** |  | [optional] 
**ApiServerUrl** | Pointer to **string** |  | [optional] 

## Methods

### NewManagedCluster

`func NewManagedCluster(name string, provider string, kubeconfigSecret string, ) *ManagedCluster`

NewManagedCluster instantiates a new ManagedCluster object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterWithDefaults

`func NewManagedClusterWithDefaults() *ManagedCluster`

NewManagedClusterWithDefaults instantiates a new ManagedCluster object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *ManagedCluster) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ManagedCluster) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ManagedCluster) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ManagedCluster) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *ManagedCluster) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ManagedCluster) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ManagedCluster) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *ManagedCluster) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *ManagedCluster) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ManagedCluster) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ManagedCluster) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *ManagedCluster) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *ManagedCluster) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ManagedCluster) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ManagedCluster) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *ManagedCluster) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *ManagedCluster) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ManagedCluster) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ManagedCluster) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *ManagedCluster) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *ManagedCluster) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ManagedCluster) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ManagedCluster) SetName(v string)`

SetName sets Name field to given value.


### GetProvider

`func (o *ManagedCluster) GetProvider() string`

GetProvider returns the Provider field if non-nil, zero value otherwise.

### GetProviderOk

`func (o *ManagedCluster) GetProviderOk() (*string, bool)`

GetProviderOk returns a tuple with the Provider field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProvider

`func (o *ManagedCluster) SetProvider(v string)`

SetProvider sets Provider field to given value.


### GetRegion

`func (o *ManagedCluster) GetRegion() string`

GetRegion returns the Region field if non-nil, zero value otherwise.

### GetRegionOk

`func (o *ManagedCluster) GetRegionOk() (*string, bool)`

GetRegionOk returns a tuple with the Region field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegion

`func (o *ManagedCluster) SetRegion(v string)`

SetRegion sets Region field to given value.

### HasRegion

`func (o *ManagedCluster) HasRegion() bool`

HasRegion returns a boolean if a field has been set.

### GetKubeconfigSecret

`func (o *ManagedCluster) GetKubeconfigSecret() string`

GetKubeconfigSecret returns the KubeconfigSecret field if non-nil, zero value otherwise.

### GetKubeconfigSecretOk

`func (o *ManagedCluster) GetKubeconfigSecretOk() (*string, bool)`

GetKubeconfigSecretOk returns a tuple with the KubeconfigSecret field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKubeconfigSecret

`func (o *ManagedCluster) SetKubeconfigSecret(v string)`

SetKubeconfigSecret sets KubeconfigSecret field to given value.


### GetStatus

`func (o *ManagedCluster) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ManagedCluster) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ManagedCluster) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *ManagedCluster) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetApiServerUrl

`func (o *ManagedCluster) GetApiServerUrl() string`

GetApiServerUrl returns the ApiServerUrl field if non-nil, zero value otherwise.

### GetApiServerUrlOk

`func (o *ManagedCluster) GetApiServerUrlOk() (*string, bool)`

GetApiServerUrlOk returns a tuple with the ApiServerUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetApiServerUrl

`func (o *ManagedCluster) SetApiServerUrl(v string)`

SetApiServerUrl sets ApiServerUrl field to given value.

### HasApiServerUrl

`func (o *ManagedCluster) HasApiServerUrl() bool`

HasApiServerUrl returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


