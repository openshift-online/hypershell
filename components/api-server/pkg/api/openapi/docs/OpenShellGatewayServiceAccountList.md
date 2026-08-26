# OpenShellGatewayServiceAccountList

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Page** | **int32** |  | 
**Size** | **int32** |  | 
**Total** | **int32** |  | 
**Capabilities** | [**OpenShellGatewayServiceAccountCapabilities**](OpenShellGatewayServiceAccountCapabilities.md) |  | 
**Items** | [**[]OpenShellGatewayServiceAccountListItem**](OpenShellGatewayServiceAccountListItem.md) |  | 

## Methods

### NewOpenShellGatewayServiceAccountList

`func NewOpenShellGatewayServiceAccountList(page int32, size int32, total int32, capabilities OpenShellGatewayServiceAccountCapabilities, items []OpenShellGatewayServiceAccountListItem, ) *OpenShellGatewayServiceAccountList`

NewOpenShellGatewayServiceAccountList instantiates a new OpenShellGatewayServiceAccountList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountListWithDefaults

`func NewOpenShellGatewayServiceAccountListWithDefaults() *OpenShellGatewayServiceAccountList`

NewOpenShellGatewayServiceAccountListWithDefaults instantiates a new OpenShellGatewayServiceAccountList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPage

`func (o *OpenShellGatewayServiceAccountList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *OpenShellGatewayServiceAccountList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *OpenShellGatewayServiceAccountList) SetPage(v int32)`

SetPage sets Page field to given value.


### GetSize

`func (o *OpenShellGatewayServiceAccountList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *OpenShellGatewayServiceAccountList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *OpenShellGatewayServiceAccountList) SetSize(v int32)`

SetSize sets Size field to given value.


### GetTotal

`func (o *OpenShellGatewayServiceAccountList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *OpenShellGatewayServiceAccountList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *OpenShellGatewayServiceAccountList) SetTotal(v int32)`

SetTotal sets Total field to given value.


### GetCapabilities

`func (o *OpenShellGatewayServiceAccountList) GetCapabilities() OpenShellGatewayServiceAccountCapabilities`

GetCapabilities returns the Capabilities field if non-nil, zero value otherwise.

### GetCapabilitiesOk

`func (o *OpenShellGatewayServiceAccountList) GetCapabilitiesOk() (*OpenShellGatewayServiceAccountCapabilities, bool)`

GetCapabilitiesOk returns a tuple with the Capabilities field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCapabilities

`func (o *OpenShellGatewayServiceAccountList) SetCapabilities(v OpenShellGatewayServiceAccountCapabilities)`

SetCapabilities sets Capabilities field to given value.


### GetItems

`func (o *OpenShellGatewayServiceAccountList) GetItems() []OpenShellGatewayServiceAccountListItem`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *OpenShellGatewayServiceAccountList) GetItemsOk() (*[]OpenShellGatewayServiceAccountListItem, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *OpenShellGatewayServiceAccountList) SetItems(v []OpenShellGatewayServiceAccountListItem)`

SetItems sets Items field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


