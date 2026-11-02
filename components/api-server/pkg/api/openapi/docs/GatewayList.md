# GatewayList

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Kind** | Pointer to **string** |  | [optional]
**Page** | Pointer to **int32** |  | [optional]
**Size** | Pointer to **int32** |  | [optional]
**Total** | Pointer to **int32** |  | [optional]
**Id** | Pointer to **string** |  | [optional]
**Href** | Pointer to **string** |  | [optional]
**CreatedAt** | Pointer to **time.Time** |  | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**Items** | Pointer to [**[]Gateway**](Gateway.md) |  | [optional]
**HasOtherOwnedGateways** | Pointer to **bool** | Browser-safe existence flag reporting whether the caller&#39;s full authorized gateway list (before search, optional filters, and the &#x60;mine&#x60; ownership filter) contains at least one gateway the caller does not own. It is computed server-side with a bounded authorization-scoped existence query rather than by enumerating pages, so a console can decide whether to show the \&quot;Show only my gateways\&quot; toggle without downloading every page. It is evaluated only over the caller&#39;s authorized gateways and never discloses the existence of gateways the caller is not authorized to see (SEC-RBAC-10). | [optional]

## Methods

### NewGatewayList

`func NewGatewayList() *GatewayList`

NewGatewayList instantiates a new GatewayList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayListWithDefaults

`func NewGatewayListWithDefaults() *GatewayList`

NewGatewayListWithDefaults instantiates a new GatewayList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *GatewayList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *GatewayList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *GatewayList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *GatewayList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *GatewayList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *GatewayList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *GatewayList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *GatewayList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *GatewayList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *GatewayList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *GatewayList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *GatewayList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *GatewayList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *GatewayList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *GatewayList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *GatewayList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *GatewayList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *GatewayList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *GatewayList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *GatewayList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *GatewayList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *GatewayList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *GatewayList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *GatewayList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *GatewayList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *GatewayList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *GatewayList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *GatewayList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *GatewayList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *GatewayList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *GatewayList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *GatewayList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *GatewayList) GetItems() []Gateway`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *GatewayList) GetItemsOk() (*[]Gateway, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *GatewayList) SetItems(v []Gateway)`

SetItems sets Items field to given value.

### HasItems

`func (o *GatewayList) HasItems() bool`

HasItems returns a boolean if a field has been set.

### GetHasOtherOwnedGateways

`func (o *GatewayList) GetHasOtherOwnedGateways() bool`

GetHasOtherOwnedGateways returns the HasOtherOwnedGateways field if non-nil, zero value otherwise.

### GetHasOtherOwnedGatewaysOk

`func (o *GatewayList) GetHasOtherOwnedGatewaysOk() (*bool, bool)`

GetHasOtherOwnedGatewaysOk returns a tuple with the HasOtherOwnedGateways field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHasOtherOwnedGateways

`func (o *GatewayList) SetHasOtherOwnedGateways(v bool)`

SetHasOtherOwnedGateways sets HasOtherOwnedGateways field to given value.

### HasHasOtherOwnedGateways

`func (o *GatewayList) HasHasOtherOwnedGateways() bool`

HasHasOtherOwnedGateways returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
