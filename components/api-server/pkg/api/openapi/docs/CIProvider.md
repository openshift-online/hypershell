# CIProvider

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional]
**Kind** | Pointer to **string** |  | [optional]
**Href** | Pointer to **string** |  | [optional]
**CreatedAt** | Pointer to **time.Time** |  | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**Forge** | **string** | The forge family: github, gitlab, or forgejo |
**Issuer** | **string** | The OIDC issuer URL whose tokens this provider validates |
**JwksUrl** | **string** | The JWKS endpoint used for signature validation |
**Audience** | **string** | The platform-pinned audience every federated CI token must carry |
**Status** | Pointer to [**CIProviderAllOfStatus**](CIProviderAllOfStatus.md) |  | [optional]

## Methods

### NewCIProvider

`func NewCIProvider(forge string, issuer string, jwksUrl string, audience string, ) *CIProvider`

NewCIProvider instantiates a new CIProvider object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCIProviderWithDefaults

`func NewCIProviderWithDefaults() *CIProvider`

NewCIProviderWithDefaults instantiates a new CIProvider object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *CIProvider) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *CIProvider) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *CIProvider) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *CIProvider) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *CIProvider) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *CIProvider) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *CIProvider) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *CIProvider) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *CIProvider) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *CIProvider) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *CIProvider) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *CIProvider) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *CIProvider) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *CIProvider) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *CIProvider) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *CIProvider) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *CIProvider) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *CIProvider) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *CIProvider) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *CIProvider) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetForge

`func (o *CIProvider) GetForge() string`

GetForge returns the Forge field if non-nil, zero value otherwise.

### GetForgeOk

`func (o *CIProvider) GetForgeOk() (*string, bool)`

GetForgeOk returns a tuple with the Forge field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetForge

`func (o *CIProvider) SetForge(v string)`

SetForge sets Forge field to given value.


### GetIssuer

`func (o *CIProvider) GetIssuer() string`

GetIssuer returns the Issuer field if non-nil, zero value otherwise.

### GetIssuerOk

`func (o *CIProvider) GetIssuerOk() (*string, bool)`

GetIssuerOk returns a tuple with the Issuer field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIssuer

`func (o *CIProvider) SetIssuer(v string)`

SetIssuer sets Issuer field to given value.


### GetJwksUrl

`func (o *CIProvider) GetJwksUrl() string`

GetJwksUrl returns the JwksUrl field if non-nil, zero value otherwise.

### GetJwksUrlOk

`func (o *CIProvider) GetJwksUrlOk() (*string, bool)`

GetJwksUrlOk returns a tuple with the JwksUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetJwksUrl

`func (o *CIProvider) SetJwksUrl(v string)`

SetJwksUrl sets JwksUrl field to given value.


### GetAudience

`func (o *CIProvider) GetAudience() string`

GetAudience returns the Audience field if non-nil, zero value otherwise.

### GetAudienceOk

`func (o *CIProvider) GetAudienceOk() (*string, bool)`

GetAudienceOk returns a tuple with the Audience field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAudience

`func (o *CIProvider) SetAudience(v string)`

SetAudience sets Audience field to given value.


### GetStatus

`func (o *CIProvider) GetStatus() CIProviderAllOfStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *CIProvider) GetStatusOk() (*CIProviderAllOfStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *CIProvider) SetStatus(v CIProviderAllOfStatus)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *CIProvider) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
