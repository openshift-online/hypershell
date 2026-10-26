# CITrust

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional]
**Kind** | Pointer to **string** |  | [optional]
**Href** | Pointer to **string** |  | [optional]
**CreatedAt** | Pointer to **time.Time** |  | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**CiProviderId** | **string** | Reference to the CIProvider whose issuer is trusted |
**GatewayId** | **string** | Reference to the Gateway this CI may operate |
**Subject** | [**CITrustAllOfSubject**](CITrustAllOfSubject.md) |  |
**Role** | **string** | The gateway role granted to the federated CI session |
**Status** | Pointer to [**CITrustAllOfStatus**](CITrustAllOfStatus.md) |  | [optional]

## Methods

### NewCITrust

`func NewCITrust(ciProviderId string, gatewayId string, subject CITrustAllOfSubject, role string, ) *CITrust`

NewCITrust instantiates a new CITrust object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCITrustWithDefaults

`func NewCITrustWithDefaults() *CITrust`

NewCITrustWithDefaults instantiates a new CITrust object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *CITrust) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *CITrust) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *CITrust) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *CITrust) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *CITrust) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *CITrust) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *CITrust) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *CITrust) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *CITrust) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *CITrust) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *CITrust) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *CITrust) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *CITrust) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *CITrust) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *CITrust) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *CITrust) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *CITrust) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *CITrust) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *CITrust) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *CITrust) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetCiProviderId

`func (o *CITrust) GetCiProviderId() string`

GetCiProviderId returns the CiProviderId field if non-nil, zero value otherwise.

### GetCiProviderIdOk

`func (o *CITrust) GetCiProviderIdOk() (*string, bool)`

GetCiProviderIdOk returns a tuple with the CiProviderId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCiProviderId

`func (o *CITrust) SetCiProviderId(v string)`

SetCiProviderId sets CiProviderId field to given value.


### GetGatewayId

`func (o *CITrust) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *CITrust) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *CITrust) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.


### GetSubject

`func (o *CITrust) GetSubject() CITrustAllOfSubject`

GetSubject returns the Subject field if non-nil, zero value otherwise.

### GetSubjectOk

`func (o *CITrust) GetSubjectOk() (*CITrustAllOfSubject, bool)`

GetSubjectOk returns a tuple with the Subject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubject

`func (o *CITrust) SetSubject(v CITrustAllOfSubject)`

SetSubject sets Subject field to given value.


### GetRole

`func (o *CITrust) GetRole() string`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *CITrust) GetRoleOk() (*string, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *CITrust) SetRole(v string)`

SetRole sets Role field to given value.


### GetStatus

`func (o *CITrust) GetStatus() CITrustAllOfStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *CITrust) GetStatusOk() (*CITrustAllOfStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *CITrust) SetStatus(v CITrustAllOfStatus)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *CITrust) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
