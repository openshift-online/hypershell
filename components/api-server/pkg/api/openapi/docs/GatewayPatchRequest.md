# GatewayPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional]
**TargetBundle** | Pointer to **string** | Release bundle to run, \&quot;&lt;bundle&gt;[#&lt;token&gt;]\&quot;. platform:admin only. The token is ignored for matching and only forces a reapply; \&quot;current\&quot; means the bundle the gateway&#39;s controller runs. | [optional]

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

### GetTargetBundle

`func (o *GatewayPatchRequest) GetTargetBundle() string`

GetTargetBundle returns the TargetBundle field if non-nil, zero value otherwise.

### GetTargetBundleOk

`func (o *GatewayPatchRequest) GetTargetBundleOk() (*string, bool)`

GetTargetBundleOk returns a tuple with the TargetBundle field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTargetBundle

`func (o *GatewayPatchRequest) SetTargetBundle(v string)`

SetTargetBundle sets TargetBundle field to given value.

### HasTargetBundle

`func (o *GatewayPatchRequest) HasTargetBundle() bool`

HasTargetBundle returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
