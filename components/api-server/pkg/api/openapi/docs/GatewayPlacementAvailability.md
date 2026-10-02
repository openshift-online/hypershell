# GatewayPlacementAvailability

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**AwsPublic** | **bool** |  | 
**AwsVpn** | **bool** |  | 
**IbmPublic** | **bool** |  | 
**IbmVpn** | **bool** |  | 
**LocalKind** | **bool** |  | 
**AwsReason** | Pointer to **string** |  | [optional] 
**IbmReason** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayPlacementAvailability

`func NewGatewayPlacementAvailability(awsPublic bool, awsVpn bool, ibmPublic bool, ibmVpn bool, localKind bool, ) *GatewayPlacementAvailability`

NewGatewayPlacementAvailability instantiates a new GatewayPlacementAvailability object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPlacementAvailabilityWithDefaults

`func NewGatewayPlacementAvailabilityWithDefaults() *GatewayPlacementAvailability`

NewGatewayPlacementAvailabilityWithDefaults instantiates a new GatewayPlacementAvailability object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAwsPublic

`func (o *GatewayPlacementAvailability) GetAwsPublic() bool`

GetAwsPublic returns the AwsPublic field if non-nil, zero value otherwise.

### GetAwsPublicOk

`func (o *GatewayPlacementAvailability) GetAwsPublicOk() (*bool, bool)`

GetAwsPublicOk returns a tuple with the AwsPublic field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAwsPublic

`func (o *GatewayPlacementAvailability) SetAwsPublic(v bool)`

SetAwsPublic sets AwsPublic field to given value.


### GetAwsVpn

`func (o *GatewayPlacementAvailability) GetAwsVpn() bool`

GetAwsVpn returns the AwsVpn field if non-nil, zero value otherwise.

### GetAwsVpnOk

`func (o *GatewayPlacementAvailability) GetAwsVpnOk() (*bool, bool)`

GetAwsVpnOk returns a tuple with the AwsVpn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAwsVpn

`func (o *GatewayPlacementAvailability) SetAwsVpn(v bool)`

SetAwsVpn sets AwsVpn field to given value.


### GetIbmPublic

`func (o *GatewayPlacementAvailability) GetIbmPublic() bool`

GetIbmPublic returns the IbmPublic field if non-nil, zero value otherwise.

### GetIbmPublicOk

`func (o *GatewayPlacementAvailability) GetIbmPublicOk() (*bool, bool)`

GetIbmPublicOk returns a tuple with the IbmPublic field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIbmPublic

`func (o *GatewayPlacementAvailability) SetIbmPublic(v bool)`

SetIbmPublic sets IbmPublic field to given value.


### GetIbmVpn

`func (o *GatewayPlacementAvailability) GetIbmVpn() bool`

GetIbmVpn returns the IbmVpn field if non-nil, zero value otherwise.

### GetIbmVpnOk

`func (o *GatewayPlacementAvailability) GetIbmVpnOk() (*bool, bool)`

GetIbmVpnOk returns a tuple with the IbmVpn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIbmVpn

`func (o *GatewayPlacementAvailability) SetIbmVpn(v bool)`

SetIbmVpn sets IbmVpn field to given value.


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


### GetAwsReason

`func (o *GatewayPlacementAvailability) GetAwsReason() string`

GetAwsReason returns the AwsReason field if non-nil, zero value otherwise.

### GetAwsReasonOk

`func (o *GatewayPlacementAvailability) GetAwsReasonOk() (*string, bool)`

GetAwsReasonOk returns a tuple with the AwsReason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAwsReason

`func (o *GatewayPlacementAvailability) SetAwsReason(v string)`

SetAwsReason sets AwsReason field to given value.

### HasAwsReason

`func (o *GatewayPlacementAvailability) HasAwsReason() bool`

HasAwsReason returns a boolean if a field has been set.

### GetIbmReason

`func (o *GatewayPlacementAvailability) GetIbmReason() string`

GetIbmReason returns the IbmReason field if non-nil, zero value otherwise.

### GetIbmReasonOk

`func (o *GatewayPlacementAvailability) GetIbmReasonOk() (*string, bool)`

GetIbmReasonOk returns a tuple with the IbmReason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIbmReason

`func (o *GatewayPlacementAvailability) SetIbmReason(v string)`

SetIbmReason sets IbmReason field to given value.

### HasIbmReason

`func (o *GatewayPlacementAvailability) HasIbmReason() bool`

HasIbmReason returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


