# GatewayPlacementIntent

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Network** | Pointer to **string** |  | [optional] 
**Provider** | Pointer to **string** | IBM Cloud supports public placement only; VPN requires AWS. | [optional] 
**Mode** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayPlacementIntent

`func NewGatewayPlacementIntent() *GatewayPlacementIntent`

NewGatewayPlacementIntent instantiates a new GatewayPlacementIntent object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPlacementIntentWithDefaults

`func NewGatewayPlacementIntentWithDefaults() *GatewayPlacementIntent`

NewGatewayPlacementIntentWithDefaults instantiates a new GatewayPlacementIntent object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetNetwork

`func (o *GatewayPlacementIntent) GetNetwork() string`

GetNetwork returns the Network field if non-nil, zero value otherwise.

### GetNetworkOk

`func (o *GatewayPlacementIntent) GetNetworkOk() (*string, bool)`

GetNetworkOk returns a tuple with the Network field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNetwork

`func (o *GatewayPlacementIntent) SetNetwork(v string)`

SetNetwork sets Network field to given value.

### HasNetwork

`func (o *GatewayPlacementIntent) HasNetwork() bool`

HasNetwork returns a boolean if a field has been set.

### GetProvider

`func (o *GatewayPlacementIntent) GetProvider() string`

GetProvider returns the Provider field if non-nil, zero value otherwise.

### GetProviderOk

`func (o *GatewayPlacementIntent) GetProviderOk() (*string, bool)`

GetProviderOk returns a tuple with the Provider field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProvider

`func (o *GatewayPlacementIntent) SetProvider(v string)`

SetProvider sets Provider field to given value.

### HasProvider

`func (o *GatewayPlacementIntent) HasProvider() bool`

HasProvider returns a boolean if a field has been set.

### GetMode

`func (o *GatewayPlacementIntent) GetMode() string`

GetMode returns the Mode field if non-nil, zero value otherwise.

### GetModeOk

`func (o *GatewayPlacementIntent) GetModeOk() (*string, bool)`

GetModeOk returns a tuple with the Mode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMode

`func (o *GatewayPlacementIntent) SetMode(v string)`

SetMode sets Mode field to given value.

### HasMode

`func (o *GatewayPlacementIntent) HasMode() bool`

HasMode returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


