# FleetDefaults

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CpuRequest** | Pointer to **string** |  | [optional]
**CpuLimit** | Pointer to **string** |  | [optional]
**MemoryRequest** | Pointer to **string** |  | [optional]
**MemoryLimit** | Pointer to **string** |  | [optional]
**CpuRequestMillis** | Pointer to **int64** |  | [optional]
**CpuLimitMillis** | Pointer to **int64** |  | [optional]
**MemoryRequestBytes** | Pointer to **int64** |  | [optional]
**MemoryLimitBytes** | Pointer to **int64** |  | [optional]

## Methods

### NewFleetDefaults

`func NewFleetDefaults() *FleetDefaults`

NewFleetDefaults instantiates a new FleetDefaults object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewFleetDefaultsWithDefaults

`func NewFleetDefaultsWithDefaults() *FleetDefaults`

NewFleetDefaultsWithDefaults instantiates a new FleetDefaults object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCpuRequest

`func (o *FleetDefaults) GetCpuRequest() string`

GetCpuRequest returns the CpuRequest field if non-nil, zero value otherwise.

### GetCpuRequestOk

`func (o *FleetDefaults) GetCpuRequestOk() (*string, bool)`

GetCpuRequestOk returns a tuple with the CpuRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequest

`func (o *FleetDefaults) SetCpuRequest(v string)`

SetCpuRequest sets CpuRequest field to given value.

### HasCpuRequest

`func (o *FleetDefaults) HasCpuRequest() bool`

HasCpuRequest returns a boolean if a field has been set.

### GetCpuLimit

`func (o *FleetDefaults) GetCpuLimit() string`

GetCpuLimit returns the CpuLimit field if non-nil, zero value otherwise.

### GetCpuLimitOk

`func (o *FleetDefaults) GetCpuLimitOk() (*string, bool)`

GetCpuLimitOk returns a tuple with the CpuLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimit

`func (o *FleetDefaults) SetCpuLimit(v string)`

SetCpuLimit sets CpuLimit field to given value.

### HasCpuLimit

`func (o *FleetDefaults) HasCpuLimit() bool`

HasCpuLimit returns a boolean if a field has been set.

### GetMemoryRequest

`func (o *FleetDefaults) GetMemoryRequest() string`

GetMemoryRequest returns the MemoryRequest field if non-nil, zero value otherwise.

### GetMemoryRequestOk

`func (o *FleetDefaults) GetMemoryRequestOk() (*string, bool)`

GetMemoryRequestOk returns a tuple with the MemoryRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequest

`func (o *FleetDefaults) SetMemoryRequest(v string)`

SetMemoryRequest sets MemoryRequest field to given value.

### HasMemoryRequest

`func (o *FleetDefaults) HasMemoryRequest() bool`

HasMemoryRequest returns a boolean if a field has been set.

### GetMemoryLimit

`func (o *FleetDefaults) GetMemoryLimit() string`

GetMemoryLimit returns the MemoryLimit field if non-nil, zero value otherwise.

### GetMemoryLimitOk

`func (o *FleetDefaults) GetMemoryLimitOk() (*string, bool)`

GetMemoryLimitOk returns a tuple with the MemoryLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimit

`func (o *FleetDefaults) SetMemoryLimit(v string)`

SetMemoryLimit sets MemoryLimit field to given value.

### HasMemoryLimit

`func (o *FleetDefaults) HasMemoryLimit() bool`

HasMemoryLimit returns a boolean if a field has been set.

### GetCpuRequestMillis

`func (o *FleetDefaults) GetCpuRequestMillis() int64`

GetCpuRequestMillis returns the CpuRequestMillis field if non-nil, zero value otherwise.

### GetCpuRequestMillisOk

`func (o *FleetDefaults) GetCpuRequestMillisOk() (*int64, bool)`

GetCpuRequestMillisOk returns a tuple with the CpuRequestMillis field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequestMillis

`func (o *FleetDefaults) SetCpuRequestMillis(v int64)`

SetCpuRequestMillis sets CpuRequestMillis field to given value.

### HasCpuRequestMillis

`func (o *FleetDefaults) HasCpuRequestMillis() bool`

HasCpuRequestMillis returns a boolean if a field has been set.

### GetCpuLimitMillis

`func (o *FleetDefaults) GetCpuLimitMillis() int64`

GetCpuLimitMillis returns the CpuLimitMillis field if non-nil, zero value otherwise.

### GetCpuLimitMillisOk

`func (o *FleetDefaults) GetCpuLimitMillisOk() (*int64, bool)`

GetCpuLimitMillisOk returns a tuple with the CpuLimitMillis field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimitMillis

`func (o *FleetDefaults) SetCpuLimitMillis(v int64)`

SetCpuLimitMillis sets CpuLimitMillis field to given value.

### HasCpuLimitMillis

`func (o *FleetDefaults) HasCpuLimitMillis() bool`

HasCpuLimitMillis returns a boolean if a field has been set.

### GetMemoryRequestBytes

`func (o *FleetDefaults) GetMemoryRequestBytes() int64`

GetMemoryRequestBytes returns the MemoryRequestBytes field if non-nil, zero value otherwise.

### GetMemoryRequestBytesOk

`func (o *FleetDefaults) GetMemoryRequestBytesOk() (*int64, bool)`

GetMemoryRequestBytesOk returns a tuple with the MemoryRequestBytes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequestBytes

`func (o *FleetDefaults) SetMemoryRequestBytes(v int64)`

SetMemoryRequestBytes sets MemoryRequestBytes field to given value.

### HasMemoryRequestBytes

`func (o *FleetDefaults) HasMemoryRequestBytes() bool`

HasMemoryRequestBytes returns a boolean if a field has been set.

### GetMemoryLimitBytes

`func (o *FleetDefaults) GetMemoryLimitBytes() int64`

GetMemoryLimitBytes returns the MemoryLimitBytes field if non-nil, zero value otherwise.

### GetMemoryLimitBytesOk

`func (o *FleetDefaults) GetMemoryLimitBytesOk() (*int64, bool)`

GetMemoryLimitBytesOk returns a tuple with the MemoryLimitBytes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimitBytes

`func (o *FleetDefaults) SetMemoryLimitBytes(v int64)`

SetMemoryLimitBytes sets MemoryLimitBytes field to given value.

### HasMemoryLimitBytes

`func (o *FleetDefaults) HasMemoryLimitBytes() bool`

HasMemoryLimitBytes returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
