# GatewayProfile

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Description** | Pointer to **string** | Profile purpose/intent | [optional] 
**CpuRequestTotal** | Pointer to **string** | Total CPU requests allowed (e.g., \&quot;4\&quot;, \&quot;500m\&quot;) | [optional] 
**CpuLimitTotal** | Pointer to **string** | Total CPU limits allowed | [optional] 
**MemoryRequestTotal** | Pointer to **string** | Total memory requests allowed (e.g., \&quot;8Gi\&quot;, \&quot;512Mi\&quot;) | [optional] 
**MemoryLimitTotal** | Pointer to **string** | Total memory limits allowed | [optional] 
**EphemeralStorageTotal** | Pointer to **string** | Total ephemeral storage allowed (e.g., \&quot;10Gi\&quot;) | [optional] 
**PodCount** | Pointer to **int32** | Maximum number of pods (0 means not set) | [optional] 
**PvcCount** | Pointer to **int32** | Maximum number of PersistentVolumeClaims (0 means not set) | [optional] 
**ContainerCpuRequestDefault** | Pointer to **string** | Default CPU request injected into containers without explicit request | [optional] 
**ContainerCpuLimitMax** | Pointer to **string** | Maximum CPU limit a container can request | [optional] 
**ContainerMemoryRequestDefault** | Pointer to **string** | Default memory request injected into containers | [optional] 
**ContainerMemoryLimitMax** | Pointer to **string** | Maximum memory limit a container can request | [optional] 

## Methods

### NewGatewayProfile

`func NewGatewayProfile(name string, ) *GatewayProfile`

NewGatewayProfile instantiates a new GatewayProfile object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayProfileWithDefaults

`func NewGatewayProfileWithDefaults() *GatewayProfile`

NewGatewayProfileWithDefaults instantiates a new GatewayProfile object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *GatewayProfile) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *GatewayProfile) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *GatewayProfile) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *GatewayProfile) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *GatewayProfile) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *GatewayProfile) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *GatewayProfile) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *GatewayProfile) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *GatewayProfile) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *GatewayProfile) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *GatewayProfile) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *GatewayProfile) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *GatewayProfile) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *GatewayProfile) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *GatewayProfile) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *GatewayProfile) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *GatewayProfile) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *GatewayProfile) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *GatewayProfile) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *GatewayProfile) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *GatewayProfile) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayProfile) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayProfile) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *GatewayProfile) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *GatewayProfile) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *GatewayProfile) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *GatewayProfile) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### GetCpuRequestTotal

`func (o *GatewayProfile) GetCpuRequestTotal() string`

GetCpuRequestTotal returns the CpuRequestTotal field if non-nil, zero value otherwise.

### GetCpuRequestTotalOk

`func (o *GatewayProfile) GetCpuRequestTotalOk() (*string, bool)`

GetCpuRequestTotalOk returns a tuple with the CpuRequestTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequestTotal

`func (o *GatewayProfile) SetCpuRequestTotal(v string)`

SetCpuRequestTotal sets CpuRequestTotal field to given value.

### HasCpuRequestTotal

`func (o *GatewayProfile) HasCpuRequestTotal() bool`

HasCpuRequestTotal returns a boolean if a field has been set.

### GetCpuLimitTotal

`func (o *GatewayProfile) GetCpuLimitTotal() string`

GetCpuLimitTotal returns the CpuLimitTotal field if non-nil, zero value otherwise.

### GetCpuLimitTotalOk

`func (o *GatewayProfile) GetCpuLimitTotalOk() (*string, bool)`

GetCpuLimitTotalOk returns a tuple with the CpuLimitTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimitTotal

`func (o *GatewayProfile) SetCpuLimitTotal(v string)`

SetCpuLimitTotal sets CpuLimitTotal field to given value.

### HasCpuLimitTotal

`func (o *GatewayProfile) HasCpuLimitTotal() bool`

HasCpuLimitTotal returns a boolean if a field has been set.

### GetMemoryRequestTotal

`func (o *GatewayProfile) GetMemoryRequestTotal() string`

GetMemoryRequestTotal returns the MemoryRequestTotal field if non-nil, zero value otherwise.

### GetMemoryRequestTotalOk

`func (o *GatewayProfile) GetMemoryRequestTotalOk() (*string, bool)`

GetMemoryRequestTotalOk returns a tuple with the MemoryRequestTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequestTotal

`func (o *GatewayProfile) SetMemoryRequestTotal(v string)`

SetMemoryRequestTotal sets MemoryRequestTotal field to given value.

### HasMemoryRequestTotal

`func (o *GatewayProfile) HasMemoryRequestTotal() bool`

HasMemoryRequestTotal returns a boolean if a field has been set.

### GetMemoryLimitTotal

`func (o *GatewayProfile) GetMemoryLimitTotal() string`

GetMemoryLimitTotal returns the MemoryLimitTotal field if non-nil, zero value otherwise.

### GetMemoryLimitTotalOk

`func (o *GatewayProfile) GetMemoryLimitTotalOk() (*string, bool)`

GetMemoryLimitTotalOk returns a tuple with the MemoryLimitTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimitTotal

`func (o *GatewayProfile) SetMemoryLimitTotal(v string)`

SetMemoryLimitTotal sets MemoryLimitTotal field to given value.

### HasMemoryLimitTotal

`func (o *GatewayProfile) HasMemoryLimitTotal() bool`

HasMemoryLimitTotal returns a boolean if a field has been set.

### GetEphemeralStorageTotal

`func (o *GatewayProfile) GetEphemeralStorageTotal() string`

GetEphemeralStorageTotal returns the EphemeralStorageTotal field if non-nil, zero value otherwise.

### GetEphemeralStorageTotalOk

`func (o *GatewayProfile) GetEphemeralStorageTotalOk() (*string, bool)`

GetEphemeralStorageTotalOk returns a tuple with the EphemeralStorageTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEphemeralStorageTotal

`func (o *GatewayProfile) SetEphemeralStorageTotal(v string)`

SetEphemeralStorageTotal sets EphemeralStorageTotal field to given value.

### HasEphemeralStorageTotal

`func (o *GatewayProfile) HasEphemeralStorageTotal() bool`

HasEphemeralStorageTotal returns a boolean if a field has been set.

### GetPodCount

`func (o *GatewayProfile) GetPodCount() int32`

GetPodCount returns the PodCount field if non-nil, zero value otherwise.

### GetPodCountOk

`func (o *GatewayProfile) GetPodCountOk() (*int32, bool)`

GetPodCountOk returns a tuple with the PodCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPodCount

`func (o *GatewayProfile) SetPodCount(v int32)`

SetPodCount sets PodCount field to given value.

### HasPodCount

`func (o *GatewayProfile) HasPodCount() bool`

HasPodCount returns a boolean if a field has been set.

### GetPvcCount

`func (o *GatewayProfile) GetPvcCount() int32`

GetPvcCount returns the PvcCount field if non-nil, zero value otherwise.

### GetPvcCountOk

`func (o *GatewayProfile) GetPvcCountOk() (*int32, bool)`

GetPvcCountOk returns a tuple with the PvcCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPvcCount

`func (o *GatewayProfile) SetPvcCount(v int32)`

SetPvcCount sets PvcCount field to given value.

### HasPvcCount

`func (o *GatewayProfile) HasPvcCount() bool`

HasPvcCount returns a boolean if a field has been set.

### GetContainerCpuRequestDefault

`func (o *GatewayProfile) GetContainerCpuRequestDefault() string`

GetContainerCpuRequestDefault returns the ContainerCpuRequestDefault field if non-nil, zero value otherwise.

### GetContainerCpuRequestDefaultOk

`func (o *GatewayProfile) GetContainerCpuRequestDefaultOk() (*string, bool)`

GetContainerCpuRequestDefaultOk returns a tuple with the ContainerCpuRequestDefault field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerCpuRequestDefault

`func (o *GatewayProfile) SetContainerCpuRequestDefault(v string)`

SetContainerCpuRequestDefault sets ContainerCpuRequestDefault field to given value.

### HasContainerCpuRequestDefault

`func (o *GatewayProfile) HasContainerCpuRequestDefault() bool`

HasContainerCpuRequestDefault returns a boolean if a field has been set.

### GetContainerCpuLimitMax

`func (o *GatewayProfile) GetContainerCpuLimitMax() string`

GetContainerCpuLimitMax returns the ContainerCpuLimitMax field if non-nil, zero value otherwise.

### GetContainerCpuLimitMaxOk

`func (o *GatewayProfile) GetContainerCpuLimitMaxOk() (*string, bool)`

GetContainerCpuLimitMaxOk returns a tuple with the ContainerCpuLimitMax field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerCpuLimitMax

`func (o *GatewayProfile) SetContainerCpuLimitMax(v string)`

SetContainerCpuLimitMax sets ContainerCpuLimitMax field to given value.

### HasContainerCpuLimitMax

`func (o *GatewayProfile) HasContainerCpuLimitMax() bool`

HasContainerCpuLimitMax returns a boolean if a field has been set.

### GetContainerMemoryRequestDefault

`func (o *GatewayProfile) GetContainerMemoryRequestDefault() string`

GetContainerMemoryRequestDefault returns the ContainerMemoryRequestDefault field if non-nil, zero value otherwise.

### GetContainerMemoryRequestDefaultOk

`func (o *GatewayProfile) GetContainerMemoryRequestDefaultOk() (*string, bool)`

GetContainerMemoryRequestDefaultOk returns a tuple with the ContainerMemoryRequestDefault field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerMemoryRequestDefault

`func (o *GatewayProfile) SetContainerMemoryRequestDefault(v string)`

SetContainerMemoryRequestDefault sets ContainerMemoryRequestDefault field to given value.

### HasContainerMemoryRequestDefault

`func (o *GatewayProfile) HasContainerMemoryRequestDefault() bool`

HasContainerMemoryRequestDefault returns a boolean if a field has been set.

### GetContainerMemoryLimitMax

`func (o *GatewayProfile) GetContainerMemoryLimitMax() string`

GetContainerMemoryLimitMax returns the ContainerMemoryLimitMax field if non-nil, zero value otherwise.

### GetContainerMemoryLimitMaxOk

`func (o *GatewayProfile) GetContainerMemoryLimitMaxOk() (*string, bool)`

GetContainerMemoryLimitMaxOk returns a tuple with the ContainerMemoryLimitMax field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerMemoryLimitMax

`func (o *GatewayProfile) SetContainerMemoryLimitMax(v string)`

SetContainerMemoryLimitMax sets ContainerMemoryLimitMax field to given value.

### HasContainerMemoryLimitMax

`func (o *GatewayProfile) HasContainerMemoryLimitMax() bool`

HasContainerMemoryLimitMax returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


