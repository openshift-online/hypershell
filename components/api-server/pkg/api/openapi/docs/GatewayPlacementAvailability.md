# GatewayPlacementAvailability

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Placements** | [**[]GatewayPlacementAvailabilityItem**](GatewayPlacementAvailabilityItem.md) |  | 
**LocalKind** | **bool** |  | 

## Methods

### NewGatewayPlacementAvailability

`func NewGatewayPlacementAvailability(placements []GatewayPlacementAvailabilityItem, localKind bool, ) *GatewayPlacementAvailability`

NewGatewayPlacementAvailability instantiates a new GatewayPlacementAvailability object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPlacementAvailabilityWithDefaults

`func NewGatewayPlacementAvailabilityWithDefaults() *GatewayPlacementAvailability`

NewGatewayPlacementAvailabilityWithDefaults instantiates a new GatewayPlacementAvailability object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPlacements

`func (o *GatewayPlacementAvailability) GetPlacements() []GatewayPlacementAvailabilityItem`

GetPlacements returns the Placements field if non-nil, zero value otherwise.

### GetPlacementsOk

`func (o *GatewayPlacementAvailability) GetPlacementsOk() (*[]GatewayPlacementAvailabilityItem, bool)`

GetPlacementsOk returns a tuple with the Placements field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPlacements

`func (o *GatewayPlacementAvailability) SetPlacements(v []GatewayPlacementAvailabilityItem)`

SetPlacements sets Placements field to given value.


### GetLocalKind

`func (o *GatewayPlacementAvailability) GetLocalKind() bool`

GetLocalKind returns the LocalKind field if non-nil, zero value otherwise.

### GetLocalKindOk

`func (o *GatewayPlacementAvailability) GetLocalKindOk() (*bool, bool)`

GetLocalKindOk returns a tuple with the LocalKind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLocalKind

`func (o *GatewayPlacementAvailability) SetLocalKind(v bool)`

SetLocalKind sets LocalKind field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


