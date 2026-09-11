# GatewayRelease

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Image** | **string** |  | 
**RolloutStrategy** | Pointer to **string** |  | [optional] 
**CanaryPercent** | Pointer to **int32** |  | [optional] 
**CanaryDuration** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayRelease

`func NewGatewayRelease(name string, image string, ) *GatewayRelease`

NewGatewayRelease instantiates a new GatewayRelease object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayReleaseWithDefaults

`func NewGatewayReleaseWithDefaults() *GatewayRelease`

NewGatewayReleaseWithDefaults instantiates a new GatewayRelease object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *GatewayRelease) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *GatewayRelease) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *GatewayRelease) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *GatewayRelease) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *GatewayRelease) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *GatewayRelease) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *GatewayRelease) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *GatewayRelease) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *GatewayRelease) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *GatewayRelease) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *GatewayRelease) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *GatewayRelease) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *GatewayRelease) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *GatewayRelease) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *GatewayRelease) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *GatewayRelease) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *GatewayRelease) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *GatewayRelease) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *GatewayRelease) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *GatewayRelease) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *GatewayRelease) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayRelease) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayRelease) SetName(v string)`

SetName sets Name field to given value.


### GetImage

`func (o *GatewayRelease) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *GatewayRelease) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *GatewayRelease) SetImage(v string)`

SetImage sets Image field to given value.


### GetRolloutStrategy

`func (o *GatewayRelease) GetRolloutStrategy() string`

GetRolloutStrategy returns the RolloutStrategy field if non-nil, zero value otherwise.

### GetRolloutStrategyOk

`func (o *GatewayRelease) GetRolloutStrategyOk() (*string, bool)`

GetRolloutStrategyOk returns a tuple with the RolloutStrategy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRolloutStrategy

`func (o *GatewayRelease) SetRolloutStrategy(v string)`

SetRolloutStrategy sets RolloutStrategy field to given value.

### HasRolloutStrategy

`func (o *GatewayRelease) HasRolloutStrategy() bool`

HasRolloutStrategy returns a boolean if a field has been set.

### GetCanaryPercent

`func (o *GatewayRelease) GetCanaryPercent() int32`

GetCanaryPercent returns the CanaryPercent field if non-nil, zero value otherwise.

### GetCanaryPercentOk

`func (o *GatewayRelease) GetCanaryPercentOk() (*int32, bool)`

GetCanaryPercentOk returns a tuple with the CanaryPercent field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanaryPercent

`func (o *GatewayRelease) SetCanaryPercent(v int32)`

SetCanaryPercent sets CanaryPercent field to given value.

### HasCanaryPercent

`func (o *GatewayRelease) HasCanaryPercent() bool`

HasCanaryPercent returns a boolean if a field has been set.

### GetCanaryDuration

`func (o *GatewayRelease) GetCanaryDuration() string`

GetCanaryDuration returns the CanaryDuration field if non-nil, zero value otherwise.

### GetCanaryDurationOk

`func (o *GatewayRelease) GetCanaryDurationOk() (*string, bool)`

GetCanaryDurationOk returns a tuple with the CanaryDuration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanaryDuration

`func (o *GatewayRelease) SetCanaryDuration(v string)`

SetCanaryDuration sets CanaryDuration field to given value.

### HasCanaryDuration

`func (o *GatewayRelease) HasCanaryDuration() bool`

HasCanaryDuration returns a boolean if a field has been set.

### GetStatus

`func (o *GatewayRelease) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *GatewayRelease) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *GatewayRelease) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *GatewayRelease) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


