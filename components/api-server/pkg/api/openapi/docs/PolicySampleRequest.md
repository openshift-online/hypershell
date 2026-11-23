# PolicySampleRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CpuRequest** | Pointer to **string** |  | [optional]
**CpuLimit** | Pointer to **string** |  | [optional]
**MemoryRequest** | Pointer to **string** |  | [optional]
**MemoryLimit** | Pointer to **string** |  | [optional]
**GpuCount** | Pointer to **int64** |  | [optional]
**Image** | Pointer to **string** |  | [optional]
**SandboxName** | Pointer to **string** |  | [optional]
**Roles** | Pointer to **[]string** |  | [optional]

## Methods

### NewPolicySampleRequest

`func NewPolicySampleRequest() *PolicySampleRequest`

NewPolicySampleRequest instantiates a new PolicySampleRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicySampleRequestWithDefaults

`func NewPolicySampleRequestWithDefaults() *PolicySampleRequest`

NewPolicySampleRequestWithDefaults instantiates a new PolicySampleRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCpuRequest

`func (o *PolicySampleRequest) GetCpuRequest() string`

GetCpuRequest returns the CpuRequest field if non-nil, zero value otherwise.

### GetCpuRequestOk

`func (o *PolicySampleRequest) GetCpuRequestOk() (*string, bool)`

GetCpuRequestOk returns a tuple with the CpuRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuRequest

`func (o *PolicySampleRequest) SetCpuRequest(v string)`

SetCpuRequest sets CpuRequest field to given value.

### HasCpuRequest

`func (o *PolicySampleRequest) HasCpuRequest() bool`

HasCpuRequest returns a boolean if a field has been set.

### GetCpuLimit

`func (o *PolicySampleRequest) GetCpuLimit() string`

GetCpuLimit returns the CpuLimit field if non-nil, zero value otherwise.

### GetCpuLimitOk

`func (o *PolicySampleRequest) GetCpuLimitOk() (*string, bool)`

GetCpuLimitOk returns a tuple with the CpuLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpuLimit

`func (o *PolicySampleRequest) SetCpuLimit(v string)`

SetCpuLimit sets CpuLimit field to given value.

### HasCpuLimit

`func (o *PolicySampleRequest) HasCpuLimit() bool`

HasCpuLimit returns a boolean if a field has been set.

### GetMemoryRequest

`func (o *PolicySampleRequest) GetMemoryRequest() string`

GetMemoryRequest returns the MemoryRequest field if non-nil, zero value otherwise.

### GetMemoryRequestOk

`func (o *PolicySampleRequest) GetMemoryRequestOk() (*string, bool)`

GetMemoryRequestOk returns a tuple with the MemoryRequest field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryRequest

`func (o *PolicySampleRequest) SetMemoryRequest(v string)`

SetMemoryRequest sets MemoryRequest field to given value.

### HasMemoryRequest

`func (o *PolicySampleRequest) HasMemoryRequest() bool`

HasMemoryRequest returns a boolean if a field has been set.

### GetMemoryLimit

`func (o *PolicySampleRequest) GetMemoryLimit() string`

GetMemoryLimit returns the MemoryLimit field if non-nil, zero value otherwise.

### GetMemoryLimitOk

`func (o *PolicySampleRequest) GetMemoryLimitOk() (*string, bool)`

GetMemoryLimitOk returns a tuple with the MemoryLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemoryLimit

`func (o *PolicySampleRequest) SetMemoryLimit(v string)`

SetMemoryLimit sets MemoryLimit field to given value.

### HasMemoryLimit

`func (o *PolicySampleRequest) HasMemoryLimit() bool`

HasMemoryLimit returns a boolean if a field has been set.

### GetGpuCount

`func (o *PolicySampleRequest) GetGpuCount() int64`

GetGpuCount returns the GpuCount field if non-nil, zero value otherwise.

### GetGpuCountOk

`func (o *PolicySampleRequest) GetGpuCountOk() (*int64, bool)`

GetGpuCountOk returns a tuple with the GpuCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGpuCount

`func (o *PolicySampleRequest) SetGpuCount(v int64)`

SetGpuCount sets GpuCount field to given value.

### HasGpuCount

`func (o *PolicySampleRequest) HasGpuCount() bool`

HasGpuCount returns a boolean if a field has been set.

### GetImage

`func (o *PolicySampleRequest) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *PolicySampleRequest) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *PolicySampleRequest) SetImage(v string)`

SetImage sets Image field to given value.

### HasImage

`func (o *PolicySampleRequest) HasImage() bool`

HasImage returns a boolean if a field has been set.

### GetSandboxName

`func (o *PolicySampleRequest) GetSandboxName() string`

GetSandboxName returns the SandboxName field if non-nil, zero value otherwise.

### GetSandboxNameOk

`func (o *PolicySampleRequest) GetSandboxNameOk() (*string, bool)`

GetSandboxNameOk returns a tuple with the SandboxName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSandboxName

`func (o *PolicySampleRequest) SetSandboxName(v string)`

SetSandboxName sets SandboxName field to given value.

### HasSandboxName

`func (o *PolicySampleRequest) HasSandboxName() bool`

HasSandboxName returns a boolean if a field has been set.

### GetRoles

`func (o *PolicySampleRequest) GetRoles() []string`

GetRoles returns the Roles field if non-nil, zero value otherwise.

### GetRolesOk

`func (o *PolicySampleRequest) GetRolesOk() (*[]string, bool)`

GetRolesOk returns a tuple with the Roles field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoles

`func (o *PolicySampleRequest) SetRoles(v []string)`

SetRoles sets Roles field to given value.

### HasRoles

`func (o *PolicySampleRequest) HasRoles() bool`

HasRoles returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
