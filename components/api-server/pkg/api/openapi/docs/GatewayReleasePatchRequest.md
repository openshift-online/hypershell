# GatewayReleasePatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**Image** | Pointer to **string** |  | [optional] 
**RolloutStrategy** | Pointer to **string** |  | [optional] 
**CanaryPercent** | Pointer to **int32** |  | [optional] 
**CanaryDuration** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayReleasePatchRequest

`func NewGatewayReleasePatchRequest() *GatewayReleasePatchRequest`

NewGatewayReleasePatchRequest instantiates a new GatewayReleasePatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayReleasePatchRequestWithDefaults

`func NewGatewayReleasePatchRequestWithDefaults() *GatewayReleasePatchRequest`

NewGatewayReleasePatchRequestWithDefaults instantiates a new GatewayReleasePatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayReleasePatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayReleasePatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayReleasePatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *GatewayReleasePatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetImage

`func (o *GatewayReleasePatchRequest) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *GatewayReleasePatchRequest) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *GatewayReleasePatchRequest) SetImage(v string)`

SetImage sets Image field to given value.

### HasImage

`func (o *GatewayReleasePatchRequest) HasImage() bool`

HasImage returns a boolean if a field has been set.

### GetRolloutStrategy

`func (o *GatewayReleasePatchRequest) GetRolloutStrategy() string`

GetRolloutStrategy returns the RolloutStrategy field if non-nil, zero value otherwise.

### GetRolloutStrategyOk

`func (o *GatewayReleasePatchRequest) GetRolloutStrategyOk() (*string, bool)`

GetRolloutStrategyOk returns a tuple with the RolloutStrategy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRolloutStrategy

`func (o *GatewayReleasePatchRequest) SetRolloutStrategy(v string)`

SetRolloutStrategy sets RolloutStrategy field to given value.

### HasRolloutStrategy

`func (o *GatewayReleasePatchRequest) HasRolloutStrategy() bool`

HasRolloutStrategy returns a boolean if a field has been set.

### GetCanaryPercent

`func (o *GatewayReleasePatchRequest) GetCanaryPercent() int32`

GetCanaryPercent returns the CanaryPercent field if non-nil, zero value otherwise.

### GetCanaryPercentOk

`func (o *GatewayReleasePatchRequest) GetCanaryPercentOk() (*int32, bool)`

GetCanaryPercentOk returns a tuple with the CanaryPercent field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanaryPercent

`func (o *GatewayReleasePatchRequest) SetCanaryPercent(v int32)`

SetCanaryPercent sets CanaryPercent field to given value.

### HasCanaryPercent

`func (o *GatewayReleasePatchRequest) HasCanaryPercent() bool`

HasCanaryPercent returns a boolean if a field has been set.

### GetCanaryDuration

`func (o *GatewayReleasePatchRequest) GetCanaryDuration() string`

GetCanaryDuration returns the CanaryDuration field if non-nil, zero value otherwise.

### GetCanaryDurationOk

`func (o *GatewayReleasePatchRequest) GetCanaryDurationOk() (*string, bool)`

GetCanaryDurationOk returns a tuple with the CanaryDuration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanaryDuration

`func (o *GatewayReleasePatchRequest) SetCanaryDuration(v string)`

SetCanaryDuration sets CanaryDuration field to given value.

### HasCanaryDuration

`func (o *GatewayReleasePatchRequest) HasCanaryDuration() bool`

HasCanaryDuration returns a boolean if a field has been set.

### GetStatus

`func (o *GatewayReleasePatchRequest) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *GatewayReleasePatchRequest) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *GatewayReleasePatchRequest) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *GatewayReleasePatchRequest) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


