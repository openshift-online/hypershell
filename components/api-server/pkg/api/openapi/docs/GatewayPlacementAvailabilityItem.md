# GatewayPlacementAvailabilityItem

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Selector** | **[]string** |  |
**Available** | **bool** |  |
**Reason** | Pointer to **string** |  | [optional]

## Methods

### NewGatewayPlacementAvailabilityItem

`func NewGatewayPlacementAvailabilityItem(selector []string, available bool, ) *GatewayPlacementAvailabilityItem`

NewGatewayPlacementAvailabilityItem instantiates a new GatewayPlacementAvailabilityItem object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPlacementAvailabilityItemWithDefaults

`func NewGatewayPlacementAvailabilityItemWithDefaults() *GatewayPlacementAvailabilityItem`

NewGatewayPlacementAvailabilityItemWithDefaults instantiates a new GatewayPlacementAvailabilityItem object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSelector

`func (o *GatewayPlacementAvailabilityItem) GetSelector() []string`

GetSelector returns the Selector field if non-nil, zero value otherwise.

### GetSelectorOk

`func (o *GatewayPlacementAvailabilityItem) GetSelectorOk() (*[]string, bool)`

GetSelectorOk returns a tuple with the Selector field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSelector

`func (o *GatewayPlacementAvailabilityItem) SetSelector(v []string)`

SetSelector sets Selector field to given value.


### GetAvailable

`func (o *GatewayPlacementAvailabilityItem) GetAvailable() bool`

GetAvailable returns the Available field if non-nil, zero value otherwise.

### GetAvailableOk

`func (o *GatewayPlacementAvailabilityItem) GetAvailableOk() (*bool, bool)`

GetAvailableOk returns a tuple with the Available field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAvailable

`func (o *GatewayPlacementAvailabilityItem) SetAvailable(v bool)`

SetAvailable sets Available field to given value.


### GetReason

`func (o *GatewayPlacementAvailabilityItem) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *GatewayPlacementAvailabilityItem) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *GatewayPlacementAvailabilityItem) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *GatewayPlacementAvailabilityItem) HasReason() bool`

HasReason returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
