# GatewayNetwork

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Topology** | Pointer to **string** |  | [optional] 
**TunnelMode** | Pointer to **string** |  | [optional] 
**HubGatewayId** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayNetwork

`func NewGatewayNetwork(name string, ) *GatewayNetwork`

NewGatewayNetwork instantiates a new GatewayNetwork object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayNetworkWithDefaults

`func NewGatewayNetworkWithDefaults() *GatewayNetwork`

NewGatewayNetworkWithDefaults instantiates a new GatewayNetwork object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *GatewayNetwork) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *GatewayNetwork) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *GatewayNetwork) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *GatewayNetwork) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *GatewayNetwork) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *GatewayNetwork) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *GatewayNetwork) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *GatewayNetwork) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *GatewayNetwork) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *GatewayNetwork) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *GatewayNetwork) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *GatewayNetwork) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *GatewayNetwork) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *GatewayNetwork) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *GatewayNetwork) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *GatewayNetwork) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *GatewayNetwork) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *GatewayNetwork) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *GatewayNetwork) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *GatewayNetwork) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *GatewayNetwork) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayNetwork) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayNetwork) SetName(v string)`

SetName sets Name field to given value.


### GetTopology

`func (o *GatewayNetwork) GetTopology() string`

GetTopology returns the Topology field if non-nil, zero value otherwise.

### GetTopologyOk

`func (o *GatewayNetwork) GetTopologyOk() (*string, bool)`

GetTopologyOk returns a tuple with the Topology field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTopology

`func (o *GatewayNetwork) SetTopology(v string)`

SetTopology sets Topology field to given value.

### HasTopology

`func (o *GatewayNetwork) HasTopology() bool`

HasTopology returns a boolean if a field has been set.

### GetTunnelMode

`func (o *GatewayNetwork) GetTunnelMode() string`

GetTunnelMode returns the TunnelMode field if non-nil, zero value otherwise.

### GetTunnelModeOk

`func (o *GatewayNetwork) GetTunnelModeOk() (*string, bool)`

GetTunnelModeOk returns a tuple with the TunnelMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTunnelMode

`func (o *GatewayNetwork) SetTunnelMode(v string)`

SetTunnelMode sets TunnelMode field to given value.

### HasTunnelMode

`func (o *GatewayNetwork) HasTunnelMode() bool`

HasTunnelMode returns a boolean if a field has been set.

### GetHubGatewayId

`func (o *GatewayNetwork) GetHubGatewayId() string`

GetHubGatewayId returns the HubGatewayId field if non-nil, zero value otherwise.

### GetHubGatewayIdOk

`func (o *GatewayNetwork) GetHubGatewayIdOk() (*string, bool)`

GetHubGatewayIdOk returns a tuple with the HubGatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHubGatewayId

`func (o *GatewayNetwork) SetHubGatewayId(v string)`

SetHubGatewayId sets HubGatewayId field to given value.

### HasHubGatewayId

`func (o *GatewayNetwork) HasHubGatewayId() bool`

HasHubGatewayId returns a boolean if a field has been set.

### GetStatus

`func (o *GatewayNetwork) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *GatewayNetwork) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *GatewayNetwork) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *GatewayNetwork) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


