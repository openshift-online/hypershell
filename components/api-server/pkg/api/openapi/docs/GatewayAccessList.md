# GatewayAccessList

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Page** | **int32** |  | 
**Size** | **int32** |  | 
**Total** | **int32** |  | 
**Capabilities** | [**GatewayAccessCapabilities**](GatewayAccessCapabilities.md) |  | 
**Items** | [**[]GatewayAccessListItem**](GatewayAccessListItem.md) |  | 

## Methods

### NewGatewayAccessList

`func NewGatewayAccessList(page int32, size int32, total int32, capabilities GatewayAccessCapabilities, items []GatewayAccessListItem, ) *GatewayAccessList`

NewGatewayAccessList instantiates a new GatewayAccessList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayAccessListWithDefaults

`func NewGatewayAccessListWithDefaults() *GatewayAccessList`

NewGatewayAccessListWithDefaults instantiates a new GatewayAccessList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPage

`func (o *GatewayAccessList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *GatewayAccessList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *GatewayAccessList) SetPage(v int32)`

SetPage sets Page field to given value.


### GetSize

`func (o *GatewayAccessList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *GatewayAccessList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *GatewayAccessList) SetSize(v int32)`

SetSize sets Size field to given value.


### GetTotal

`func (o *GatewayAccessList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *GatewayAccessList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *GatewayAccessList) SetTotal(v int32)`

SetTotal sets Total field to given value.


### GetCapabilities

`func (o *GatewayAccessList) GetCapabilities() GatewayAccessCapabilities`

GetCapabilities returns the Capabilities field if non-nil, zero value otherwise.

### GetCapabilitiesOk

`func (o *GatewayAccessList) GetCapabilitiesOk() (*GatewayAccessCapabilities, bool)`

GetCapabilitiesOk returns a tuple with the Capabilities field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCapabilities

`func (o *GatewayAccessList) SetCapabilities(v GatewayAccessCapabilities)`

SetCapabilities sets Capabilities field to given value.


### GetItems

`func (o *GatewayAccessList) GetItems() []GatewayAccessListItem`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *GatewayAccessList) GetItemsOk() (*[]GatewayAccessListItem, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *GatewayAccessList) SetItems(v []GatewayAccessListItem)`

SetItems sets Items field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


