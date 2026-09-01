# ManagedDatabase

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
**Namespace** | Pointer to **string** |  | [optional] [readonly] 
**Region** | Pointer to **string** |  | [optional] 
**Engine** | Pointer to **string** |  | [optional] 
**EngineVersion** | Pointer to **string** |  | [optional] 
**InstanceClass** | Pointer to **string** |  | [optional] 
**ConnectionSecret** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 

## Methods

### NewManagedDatabase

`func NewManagedDatabase(name string, provider string, ) *ManagedDatabase`

NewManagedDatabase instantiates a new ManagedDatabase object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedDatabaseWithDefaults

`func NewManagedDatabaseWithDefaults() *ManagedDatabase`

NewManagedDatabaseWithDefaults instantiates a new ManagedDatabase object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *ManagedDatabase) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ManagedDatabase) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ManagedDatabase) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ManagedDatabase) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *ManagedDatabase) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ManagedDatabase) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ManagedDatabase) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *ManagedDatabase) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *ManagedDatabase) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ManagedDatabase) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ManagedDatabase) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *ManagedDatabase) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *ManagedDatabase) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ManagedDatabase) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ManagedDatabase) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *ManagedDatabase) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *ManagedDatabase) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ManagedDatabase) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ManagedDatabase) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *ManagedDatabase) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *ManagedDatabase) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ManagedDatabase) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ManagedDatabase) SetName(v string)`

SetName sets Name field to given value.


### GetProvider

`func (o *ManagedDatabase) GetProvider() string`

GetProvider returns the Provider field if non-nil, zero value otherwise.

### GetProviderOk

`func (o *ManagedDatabase) GetProviderOk() (*string, bool)`

GetProviderOk returns a tuple with the Provider field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProvider

`func (o *ManagedDatabase) SetProvider(v string)`

SetProvider sets Provider field to given value.


### GetNamespace

`func (o *ManagedDatabase) GetNamespace() string`

GetNamespace returns the Namespace field if non-nil, zero value otherwise.

### GetNamespaceOk

`func (o *ManagedDatabase) GetNamespaceOk() (*string, bool)`

GetNamespaceOk returns a tuple with the Namespace field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNamespace

`func (o *ManagedDatabase) SetNamespace(v string)`

SetNamespace sets Namespace field to given value.

### HasNamespace

`func (o *ManagedDatabase) HasNamespace() bool`

HasNamespace returns a boolean if a field has been set.

### GetRegion

`func (o *ManagedDatabase) GetRegion() string`

GetRegion returns the Region field if non-nil, zero value otherwise.

### GetRegionOk

`func (o *ManagedDatabase) GetRegionOk() (*string, bool)`

GetRegionOk returns a tuple with the Region field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegion

`func (o *ManagedDatabase) SetRegion(v string)`

SetRegion sets Region field to given value.

### HasRegion

`func (o *ManagedDatabase) HasRegion() bool`

HasRegion returns a boolean if a field has been set.

### GetEngine

`func (o *ManagedDatabase) GetEngine() string`

GetEngine returns the Engine field if non-nil, zero value otherwise.

### GetEngineOk

`func (o *ManagedDatabase) GetEngineOk() (*string, bool)`

GetEngineOk returns a tuple with the Engine field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEngine

`func (o *ManagedDatabase) SetEngine(v string)`

SetEngine sets Engine field to given value.

### HasEngine

`func (o *ManagedDatabase) HasEngine() bool`

HasEngine returns a boolean if a field has been set.

### GetEngineVersion

`func (o *ManagedDatabase) GetEngineVersion() string`

GetEngineVersion returns the EngineVersion field if non-nil, zero value otherwise.

### GetEngineVersionOk

`func (o *ManagedDatabase) GetEngineVersionOk() (*string, bool)`

GetEngineVersionOk returns a tuple with the EngineVersion field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEngineVersion

`func (o *ManagedDatabase) SetEngineVersion(v string)`

SetEngineVersion sets EngineVersion field to given value.

### HasEngineVersion

`func (o *ManagedDatabase) HasEngineVersion() bool`

HasEngineVersion returns a boolean if a field has been set.

### GetInstanceClass

`func (o *ManagedDatabase) GetInstanceClass() string`

GetInstanceClass returns the InstanceClass field if non-nil, zero value otherwise.

### GetInstanceClassOk

`func (o *ManagedDatabase) GetInstanceClassOk() (*string, bool)`

GetInstanceClassOk returns a tuple with the InstanceClass field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInstanceClass

`func (o *ManagedDatabase) SetInstanceClass(v string)`

SetInstanceClass sets InstanceClass field to given value.

### HasInstanceClass

`func (o *ManagedDatabase) HasInstanceClass() bool`

HasInstanceClass returns a boolean if a field has been set.

### GetConnectionSecret

`func (o *ManagedDatabase) GetConnectionSecret() string`

GetConnectionSecret returns the ConnectionSecret field if non-nil, zero value otherwise.

### GetConnectionSecretOk

`func (o *ManagedDatabase) GetConnectionSecretOk() (*string, bool)`

GetConnectionSecretOk returns a tuple with the ConnectionSecret field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConnectionSecret

`func (o *ManagedDatabase) SetConnectionSecret(v string)`

SetConnectionSecret sets ConnectionSecret field to given value.

### HasConnectionSecret

`func (o *ManagedDatabase) HasConnectionSecret() bool`

HasConnectionSecret returns a boolean if a field has been set.

### GetStatus

`func (o *ManagedDatabase) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ManagedDatabase) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ManagedDatabase) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *ManagedDatabase) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


