# GatewayPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional]
**AdditionalCaBundle** | Pointer to **NullableString** | User-supplied PEM bundle of additional destination CA certificates for sandbox egress to private-PKI destinations; empty means default trust roots. Set to null or empty string to clear the bundle. User-writable on update (PLAT-DATA-23 allowlist exception) | [optional]

## Methods

### NewGatewayPatchRequest

`func NewGatewayPatchRequest() *GatewayPatchRequest`

NewGatewayPatchRequest instantiates a new GatewayPatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPatchRequestWithDefaults

`func NewGatewayPatchRequestWithDefaults() *GatewayPatchRequest`

NewGatewayPatchRequestWithDefaults instantiates a new GatewayPatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayPatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayPatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayPatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *GatewayPatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetAdditionalCaBundle

`func (o *GatewayPatchRequest) GetAdditionalCaBundle() string`

GetAdditionalCaBundle returns the AdditionalCaBundle field if non-nil, zero value otherwise.

### GetAdditionalCaBundleOk

`func (o *GatewayPatchRequest) GetAdditionalCaBundleOk() (*string, bool)`

GetAdditionalCaBundleOk returns a tuple with the AdditionalCaBundle field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAdditionalCaBundle

`func (o *GatewayPatchRequest) SetAdditionalCaBundle(v string)`

SetAdditionalCaBundle sets AdditionalCaBundle field to given value.

### HasAdditionalCaBundle

`func (o *GatewayPatchRequest) HasAdditionalCaBundle() bool`

HasAdditionalCaBundle returns a boolean if a field has been set.

### SetAdditionalCaBundleNil

`func (o *GatewayPatchRequest) SetAdditionalCaBundleNil(b bool)`

 SetAdditionalCaBundleNil sets the value for AdditionalCaBundle to be an explicit nil

### UnsetAdditionalCaBundle
`func (o *GatewayPatchRequest) UnsetAdditionalCaBundle()`

UnsetAdditionalCaBundle ensures that no value is present for AdditionalCaBundle, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
