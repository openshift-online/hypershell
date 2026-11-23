# PlatformPolicySettingsDefaults

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

### NewPlatformPolicySettingsDefaults

`func NewPlatformPolicySettingsDefaults() *PlatformPolicySettingsDefaults`

NewPlatformPolicySettingsDefaults instantiates a new PlatformPolicySettingsDefaults object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformPolicySettingsDefaultsWithDefaults

`func NewPlatformPolicySettingsDefaultsWithDefaults() *PlatformPolicySettingsDefaults`

NewPlatformPolicySettingsDefaultsWithDefaults instantiates a new PlatformPolicySettingsDefaults object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCpuRequest

`func (o *PlatformPolicySettingsDefaults) GetCpuRequest() string`

GetCpuRequest returns the CpuRequest field if non-nil, zero value otherwise.

### GetCpuRequestOk

`func (o *PlatformPolicySettingsDefaults) GetCpuRequestOk() (*string, bool)`

GetCpuRequestOk returns a tuple with the CpuRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequest

`func (o *PlatformPolicySettingsDefaults) SetCpuRequest(v string)`

SetCpuRequest sets CpuRequest field to given value.

### HasCpuRequest

`func (o *PlatformPolicySettingsDefaults) HasCpuRequest() bool`

HasCpuRequest returns a boolean if a field has been set.

### GetCpuLimit

`func (o *PlatformPolicySettingsDefaults) GetCpuLimit() string`

GetCpuLimit returns the CpuLimit field if non-nil, zero value otherwise.

### GetCpuLimitOk

`func (o *PlatformPolicySettingsDefaults) GetCpuLimitOk() (*string, bool)`

GetCpuLimitOk returns a tuple with the CpuLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimit

`func (o *PlatformPolicySettingsDefaults) SetCpuLimit(v string)`

SetCpuLimit sets CpuLimit field to given value.

### HasCpuLimit

`func (o *PlatformPolicySettingsDefaults) HasCpuLimit() bool`

HasCpuLimit returns a boolean if a field has been set.

### GetMemoryRequest

`func (o *PlatformPolicySettingsDefaults) GetMemoryRequest() string`

GetMemoryRequest returns the MemoryRequest field if non-nil, zero value otherwise.

### GetMemoryRequestOk

`func (o *PlatformPolicySettingsDefaults) GetMemoryRequestOk() (*string, bool)`

GetMemoryRequestOk returns a tuple with the MemoryRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequest

`func (o *PlatformPolicySettingsDefaults) SetMemoryRequest(v string)`

SetMemoryRequest sets MemoryRequest field to given value.

### HasMemoryRequest

`func (o *PlatformPolicySettingsDefaults) HasMemoryRequest() bool`

HasMemoryRequest returns a boolean if a field has been set.

### GetMemoryLimit

`func (o *PlatformPolicySettingsDefaults) GetMemoryLimit() string`

GetMemoryLimit returns the MemoryLimit field if non-nil, zero value otherwise.

### GetMemoryLimitOk

`func (o *PlatformPolicySettingsDefaults) GetMemoryLimitOk() (*string, bool)`

GetMemoryLimitOk returns a tuple with the MemoryLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimit

`func (o *PlatformPolicySettingsDefaults) SetMemoryLimit(v string)`

SetMemoryLimit sets MemoryLimit field to given value.

### HasMemoryLimit

`func (o *PlatformPolicySettingsDefaults) HasMemoryLimit() bool`

HasMemoryLimit returns a boolean if a field has been set.

### GetCpuRequestMillis

`func (o *PlatformPolicySettingsDefaults) GetCpuRequestMillis() int64`

GetCpuRequestMillis returns the CpuRequestMillis field if non-nil, zero value otherwise.

### GetCpuRequestMillisOk

`func (o *PlatformPolicySettingsDefaults) GetCpuRequestMillisOk() (*int64, bool)`

GetCpuRequestMillisOk returns a tuple with the CpuRequestMillis field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequestMillis

`func (o *PlatformPolicySettingsDefaults) SetCpuRequestMillis(v int64)`

SetCpuRequestMillis sets CpuRequestMillis field to given value.

### HasCpuRequestMillis

`func (o *PlatformPolicySettingsDefaults) HasCpuRequestMillis() bool`

HasCpuRequestMillis returns a boolean if a field has been set.

### GetCpuLimitMillis

`func (o *PlatformPolicySettingsDefaults) GetCpuLimitMillis() int64`

GetCpuLimitMillis returns the CpuLimitMillis field if non-nil, zero value otherwise.

### GetCpuLimitMillisOk

`func (o *PlatformPolicySettingsDefaults) GetCpuLimitMillisOk() (*int64, bool)`

GetCpuLimitMillisOk returns a tuple with the CpuLimitMillis field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimitMillis

`func (o *PlatformPolicySettingsDefaults) SetCpuLimitMillis(v int64)`

SetCpuLimitMillis sets CpuLimitMillis field to given value.

### HasCpuLimitMillis

`func (o *PlatformPolicySettingsDefaults) HasCpuLimitMillis() bool`

HasCpuLimitMillis returns a boolean if a field has been set.

### GetMemoryRequestBytes

`func (o *PlatformPolicySettingsDefaults) GetMemoryRequestBytes() int64`

GetMemoryRequestBytes returns the MemoryRequestBytes field if non-nil, zero value otherwise.

### GetMemoryRequestBytesOk

`func (o *PlatformPolicySettingsDefaults) GetMemoryRequestBytesOk() (*int64, bool)`

GetMemoryRequestBytesOk returns a tuple with the MemoryRequestBytes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequestBytes

`func (o *PlatformPolicySettingsDefaults) SetMemoryRequestBytes(v int64)`

SetMemoryRequestBytes sets MemoryRequestBytes field to given value.

### HasMemoryRequestBytes

`func (o *PlatformPolicySettingsDefaults) HasMemoryRequestBytes() bool`

HasMemoryRequestBytes returns a boolean if a field has been set.

### GetMemoryLimitBytes

`func (o *PlatformPolicySettingsDefaults) GetMemoryLimitBytes() int64`

GetMemoryLimitBytes returns the MemoryLimitBytes field if non-nil, zero value otherwise.

### GetMemoryLimitBytesOk

`func (o *PlatformPolicySettingsDefaults) GetMemoryLimitBytesOk() (*int64, bool)`

GetMemoryLimitBytesOk returns a tuple with the MemoryLimitBytes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimitBytes

`func (o *PlatformPolicySettingsDefaults) SetMemoryLimitBytes(v int64)`

SetMemoryLimitBytes sets MemoryLimitBytes field to given value.

### HasMemoryLimitBytes

`func (o *PlatformPolicySettingsDefaults) HasMemoryLimitBytes() bool`

HasMemoryLimitBytes returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
