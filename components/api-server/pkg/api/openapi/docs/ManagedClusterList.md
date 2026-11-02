# ManagedClusterList

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
**Items** | Pointer to [**[]ManagedCluster**](ManagedCluster.md) |  | [optional]

## Methods

### NewManagedClusterList

`func NewManagedClusterList() *ManagedClusterList`

NewManagedClusterList instantiates a new ManagedClusterList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterListWithDefaults

`func NewManagedClusterListWithDefaults() *ManagedClusterList`

NewManagedClusterListWithDefaults instantiates a new ManagedClusterList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *ManagedClusterList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ManagedClusterList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ManagedClusterList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *ManagedClusterList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *ManagedClusterList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *ManagedClusterList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *ManagedClusterList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *ManagedClusterList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *ManagedClusterList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *ManagedClusterList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *ManagedClusterList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *ManagedClusterList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *ManagedClusterList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *ManagedClusterList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *ManagedClusterList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *ManagedClusterList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *ManagedClusterList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ManagedClusterList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ManagedClusterList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ManagedClusterList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *ManagedClusterList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ManagedClusterList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ManagedClusterList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *ManagedClusterList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *ManagedClusterList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ManagedClusterList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ManagedClusterList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *ManagedClusterList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *ManagedClusterList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ManagedClusterList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ManagedClusterList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *ManagedClusterList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *ManagedClusterList) GetItems() []ManagedCluster`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *ManagedClusterList) GetItemsOk() (*[]ManagedCluster, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *ManagedClusterList) SetItems(v []ManagedCluster)`

SetItems sets Items field to given value.

### HasItems

`func (o *ManagedClusterList) HasItems() bool`

HasItems returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
