# SecretSourceList

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
**Items** | Pointer to [**[]SecretSource**](SecretSource.md) |  | [optional] 

## Methods

### NewSecretSourceList

`func NewSecretSourceList() *SecretSourceList`

NewSecretSourceList instantiates a new SecretSourceList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSecretSourceListWithDefaults

`func NewSecretSourceListWithDefaults() *SecretSourceList`

NewSecretSourceListWithDefaults instantiates a new SecretSourceList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *SecretSourceList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *SecretSourceList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *SecretSourceList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *SecretSourceList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *SecretSourceList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *SecretSourceList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *SecretSourceList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *SecretSourceList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *SecretSourceList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *SecretSourceList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *SecretSourceList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *SecretSourceList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *SecretSourceList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *SecretSourceList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *SecretSourceList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *SecretSourceList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *SecretSourceList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *SecretSourceList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *SecretSourceList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *SecretSourceList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *SecretSourceList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *SecretSourceList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *SecretSourceList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *SecretSourceList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *SecretSourceList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *SecretSourceList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *SecretSourceList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *SecretSourceList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *SecretSourceList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *SecretSourceList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *SecretSourceList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *SecretSourceList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *SecretSourceList) GetItems() []SecretSource`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *SecretSourceList) GetItemsOk() (*[]SecretSource, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *SecretSourceList) SetItems(v []SecretSource)`

SetItems sets Items field to given value.

### HasItems

`func (o *SecretSourceList) HasItems() bool`

HasItems returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


