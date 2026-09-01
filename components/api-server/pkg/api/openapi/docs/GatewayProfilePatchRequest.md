# GatewayProfilePatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**Description** | Pointer to **string** |  | [optional] 
**CpuRequestTotal** | Pointer to **string** |  | [optional] 
**CpuLimitTotal** | Pointer to **string** |  | [optional] 
**MemoryRequestTotal** | Pointer to **string** |  | [optional] 
**MemoryLimitTotal** | Pointer to **string** |  | [optional] 
**EphemeralStorageTotal** | Pointer to **string** |  | [optional] 
**PodCount** | Pointer to **int32** |  | [optional] 
**PvcCount** | Pointer to **int32** |  | [optional] 
**ContainerCpuRequestDefault** | Pointer to **string** |  | [optional] 
**ContainerCpuLimitMax** | Pointer to **string** |  | [optional] 
**ContainerMemoryRequestDefault** | Pointer to **string** |  | [optional] 
**ContainerMemoryLimitMax** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayProfilePatchRequest

`func NewGatewayProfilePatchRequest() *GatewayProfilePatchRequest`

NewGatewayProfilePatchRequest instantiates a new GatewayProfilePatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayProfilePatchRequestWithDefaults

`func NewGatewayProfilePatchRequestWithDefaults() *GatewayProfilePatchRequest`

NewGatewayProfilePatchRequestWithDefaults instantiates a new GatewayProfilePatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayProfilePatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayProfilePatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayProfilePatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *GatewayProfilePatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetDescription

`func (o *GatewayProfilePatchRequest) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *GatewayProfilePatchRequest) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *GatewayProfilePatchRequest) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *GatewayProfilePatchRequest) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### GetCpuRequestTotal

`func (o *GatewayProfilePatchRequest) GetCpuRequestTotal() string`

GetCpuRequestTotal returns the CpuRequestTotal field if non-nil, zero value otherwise.

### GetCpuRequestTotalOk

`func (o *GatewayProfilePatchRequest) GetCpuRequestTotalOk() (*string, bool)`

GetCpuRequestTotalOk returns a tuple with the CpuRequestTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequestTotal

`func (o *GatewayProfilePatchRequest) SetCpuRequestTotal(v string)`

SetCpuRequestTotal sets CpuRequestTotal field to given value.

### HasCpuRequestTotal

`func (o *GatewayProfilePatchRequest) HasCpuRequestTotal() bool`

HasCpuRequestTotal returns a boolean if a field has been set.

### GetCpuLimitTotal

`func (o *GatewayProfilePatchRequest) GetCpuLimitTotal() string`

GetCpuLimitTotal returns the CpuLimitTotal field if non-nil, zero value otherwise.

### GetCpuLimitTotalOk

`func (o *GatewayProfilePatchRequest) GetCpuLimitTotalOk() (*string, bool)`

GetCpuLimitTotalOk returns a tuple with the CpuLimitTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimitTotal

`func (o *GatewayProfilePatchRequest) SetCpuLimitTotal(v string)`

SetCpuLimitTotal sets CpuLimitTotal field to given value.

### HasCpuLimitTotal

`func (o *GatewayProfilePatchRequest) HasCpuLimitTotal() bool`

HasCpuLimitTotal returns a boolean if a field has been set.

### GetMemoryRequestTotal

`func (o *GatewayProfilePatchRequest) GetMemoryRequestTotal() string`

GetMemoryRequestTotal returns the MemoryRequestTotal field if non-nil, zero value otherwise.

### GetMemoryRequestTotalOk

`func (o *GatewayProfilePatchRequest) GetMemoryRequestTotalOk() (*string, bool)`

GetMemoryRequestTotalOk returns a tuple with the MemoryRequestTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequestTotal

`func (o *GatewayProfilePatchRequest) SetMemoryRequestTotal(v string)`

SetMemoryRequestTotal sets MemoryRequestTotal field to given value.

### HasMemoryRequestTotal

`func (o *GatewayProfilePatchRequest) HasMemoryRequestTotal() bool`

HasMemoryRequestTotal returns a boolean if a field has been set.

### GetMemoryLimitTotal

`func (o *GatewayProfilePatchRequest) GetMemoryLimitTotal() string`

GetMemoryLimitTotal returns the MemoryLimitTotal field if non-nil, zero value otherwise.

### GetMemoryLimitTotalOk

`func (o *GatewayProfilePatchRequest) GetMemoryLimitTotalOk() (*string, bool)`

GetMemoryLimitTotalOk returns a tuple with the MemoryLimitTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimitTotal

`func (o *GatewayProfilePatchRequest) SetMemoryLimitTotal(v string)`

SetMemoryLimitTotal sets MemoryLimitTotal field to given value.

### HasMemoryLimitTotal

`func (o *GatewayProfilePatchRequest) HasMemoryLimitTotal() bool`

HasMemoryLimitTotal returns a boolean if a field has been set.

### GetEphemeralStorageTotal

`func (o *GatewayProfilePatchRequest) GetEphemeralStorageTotal() string`

GetEphemeralStorageTotal returns the EphemeralStorageTotal field if non-nil, zero value otherwise.

### GetEphemeralStorageTotalOk

`func (o *GatewayProfilePatchRequest) GetEphemeralStorageTotalOk() (*string, bool)`

GetEphemeralStorageTotalOk returns a tuple with the EphemeralStorageTotal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEphemeralStorageTotal

`func (o *GatewayProfilePatchRequest) SetEphemeralStorageTotal(v string)`

SetEphemeralStorageTotal sets EphemeralStorageTotal field to given value.

### HasEphemeralStorageTotal

`func (o *GatewayProfilePatchRequest) HasEphemeralStorageTotal() bool`

HasEphemeralStorageTotal returns a boolean if a field has been set.

### GetPodCount

`func (o *GatewayProfilePatchRequest) GetPodCount() int32`

GetPodCount returns the PodCount field if non-nil, zero value otherwise.

### GetPodCountOk

`func (o *GatewayProfilePatchRequest) GetPodCountOk() (*int32, bool)`

GetPodCountOk returns a tuple with the PodCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPodCount

`func (o *GatewayProfilePatchRequest) SetPodCount(v int32)`

SetPodCount sets PodCount field to given value.

### HasPodCount

`func (o *GatewayProfilePatchRequest) HasPodCount() bool`

HasPodCount returns a boolean if a field has been set.

### GetPvcCount

`func (o *GatewayProfilePatchRequest) GetPvcCount() int32`

GetPvcCount returns the PvcCount field if non-nil, zero value otherwise.

### GetPvcCountOk

`func (o *GatewayProfilePatchRequest) GetPvcCountOk() (*int32, bool)`

GetPvcCountOk returns a tuple with the PvcCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPvcCount

`func (o *GatewayProfilePatchRequest) SetPvcCount(v int32)`

SetPvcCount sets PvcCount field to given value.

### HasPvcCount

`func (o *GatewayProfilePatchRequest) HasPvcCount() bool`

HasPvcCount returns a boolean if a field has been set.

### GetContainerCpuRequestDefault

`func (o *GatewayProfilePatchRequest) GetContainerCpuRequestDefault() string`

GetContainerCpuRequestDefault returns the ContainerCpuRequestDefault field if non-nil, zero value otherwise.

### GetContainerCpuRequestDefaultOk

`func (o *GatewayProfilePatchRequest) GetContainerCpuRequestDefaultOk() (*string, bool)`

GetContainerCpuRequestDefaultOk returns a tuple with the ContainerCpuRequestDefault field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerCpuRequestDefault

`func (o *GatewayProfilePatchRequest) SetContainerCpuRequestDefault(v string)`

SetContainerCpuRequestDefault sets ContainerCpuRequestDefault field to given value.

### HasContainerCpuRequestDefault

`func (o *GatewayProfilePatchRequest) HasContainerCpuRequestDefault() bool`

HasContainerCpuRequestDefault returns a boolean if a field has been set.

### GetContainerCpuLimitMax

`func (o *GatewayProfilePatchRequest) GetContainerCpuLimitMax() string`

GetContainerCpuLimitMax returns the ContainerCpuLimitMax field if non-nil, zero value otherwise.

### GetContainerCpuLimitMaxOk

`func (o *GatewayProfilePatchRequest) GetContainerCpuLimitMaxOk() (*string, bool)`

GetContainerCpuLimitMaxOk returns a tuple with the ContainerCpuLimitMax field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerCpuLimitMax

`func (o *GatewayProfilePatchRequest) SetContainerCpuLimitMax(v string)`

SetContainerCpuLimitMax sets ContainerCpuLimitMax field to given value.

### HasContainerCpuLimitMax

`func (o *GatewayProfilePatchRequest) HasContainerCpuLimitMax() bool`

HasContainerCpuLimitMax returns a boolean if a field has been set.

### GetContainerMemoryRequestDefault

`func (o *GatewayProfilePatchRequest) GetContainerMemoryRequestDefault() string`

GetContainerMemoryRequestDefault returns the ContainerMemoryRequestDefault field if non-nil, zero value otherwise.

### GetContainerMemoryRequestDefaultOk

`func (o *GatewayProfilePatchRequest) GetContainerMemoryRequestDefaultOk() (*string, bool)`

GetContainerMemoryRequestDefaultOk returns a tuple with the ContainerMemoryRequestDefault field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerMemoryRequestDefault

`func (o *GatewayProfilePatchRequest) SetContainerMemoryRequestDefault(v string)`

SetContainerMemoryRequestDefault sets ContainerMemoryRequestDefault field to given value.

### HasContainerMemoryRequestDefault

`func (o *GatewayProfilePatchRequest) HasContainerMemoryRequestDefault() bool`

HasContainerMemoryRequestDefault returns a boolean if a field has been set.

### GetContainerMemoryLimitMax

`func (o *GatewayProfilePatchRequest) GetContainerMemoryLimitMax() string`

GetContainerMemoryLimitMax returns the ContainerMemoryLimitMax field if non-nil, zero value otherwise.

### GetContainerMemoryLimitMaxOk

`func (o *GatewayProfilePatchRequest) GetContainerMemoryLimitMaxOk() (*string, bool)`

GetContainerMemoryLimitMaxOk returns a tuple with the ContainerMemoryLimitMax field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContainerMemoryLimitMax

`func (o *GatewayProfilePatchRequest) SetContainerMemoryLimitMax(v string)`

SetContainerMemoryLimitMax sets ContainerMemoryLimitMax field to given value.

### HasContainerMemoryLimitMax

`func (o *GatewayProfilePatchRequest) HasContainerMemoryLimitMax() bool`

HasContainerMemoryLimitMax returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


