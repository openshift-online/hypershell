# CITrustPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Subject** | Pointer to [**CITrustPatchRequestSubject**](CITrustPatchRequestSubject.md) |  | [optional]
**Role** | Pointer to **string** |  | [optional]

## Methods

### NewCITrustPatchRequest

`func NewCITrustPatchRequest() *CITrustPatchRequest`

NewCITrustPatchRequest instantiates a new CITrustPatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCITrustPatchRequestWithDefaults

`func NewCITrustPatchRequestWithDefaults() *CITrustPatchRequest`

NewCITrustPatchRequestWithDefaults instantiates a new CITrustPatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSubject

`func (o *CITrustPatchRequest) GetSubject() CITrustPatchRequestSubject`

GetSubject returns the Subject field if non-nil, zero value otherwise.

### GetSubjectOk

`func (o *CITrustPatchRequest) GetSubjectOk() (*CITrustPatchRequestSubject, bool)`

GetSubjectOk returns a tuple with the Subject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubject

`func (o *CITrustPatchRequest) SetSubject(v CITrustPatchRequestSubject)`

SetSubject sets Subject field to given value.

### HasSubject

`func (o *CITrustPatchRequest) HasSubject() bool`

HasSubject returns a boolean if a field has been set.

### GetRole

`func (o *CITrustPatchRequest) GetRole() string`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *CITrustPatchRequest) GetRoleOk() (*string, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *CITrustPatchRequest) SetRole(v string)`

SetRole sets Role field to given value.

### HasRole

`func (o *CITrustPatchRequest) HasRole() bool`

HasRole returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
