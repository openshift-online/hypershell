# ProviderSpecList

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
**Items** | Pointer to [**[]ProviderSpec**](ProviderSpec.md) |  | [optional] 

## Methods

### NewProviderSpecList

`func NewProviderSpecList() *ProviderSpecList`

NewProviderSpecList instantiates a new ProviderSpecList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewProviderSpecListWithDefaults

`func NewProviderSpecListWithDefaults() *ProviderSpecList`

NewProviderSpecListWithDefaults instantiates a new ProviderSpecList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *ProviderSpecList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ProviderSpecList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ProviderSpecList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *ProviderSpecList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *ProviderSpecList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *ProviderSpecList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *ProviderSpecList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *ProviderSpecList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *ProviderSpecList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *ProviderSpecList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *ProviderSpecList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *ProviderSpecList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *ProviderSpecList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *ProviderSpecList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *ProviderSpecList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *ProviderSpecList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *ProviderSpecList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ProviderSpecList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ProviderSpecList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ProviderSpecList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *ProviderSpecList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ProviderSpecList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ProviderSpecList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *ProviderSpecList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *ProviderSpecList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ProviderSpecList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ProviderSpecList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *ProviderSpecList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *ProviderSpecList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ProviderSpecList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ProviderSpecList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *ProviderSpecList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *ProviderSpecList) GetItems() []ProviderSpec`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *ProviderSpecList) GetItemsOk() (*[]ProviderSpec, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *ProviderSpecList) SetItems(v []ProviderSpec)`

SetItems sets Items field to given value.

### HasItems

`func (o *ProviderSpecList) HasItems() bool`

HasItems returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


