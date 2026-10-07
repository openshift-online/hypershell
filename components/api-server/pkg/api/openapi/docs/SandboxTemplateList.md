# SandboxTemplateList

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
**Items** | Pointer to [**[]SandboxTemplate**](SandboxTemplate.md) |  | [optional] 

## Methods

### NewSandboxTemplateList

`func NewSandboxTemplateList() *SandboxTemplateList`

NewSandboxTemplateList instantiates a new SandboxTemplateList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSandboxTemplateListWithDefaults

`func NewSandboxTemplateListWithDefaults() *SandboxTemplateList`

NewSandboxTemplateListWithDefaults instantiates a new SandboxTemplateList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *SandboxTemplateList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *SandboxTemplateList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *SandboxTemplateList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *SandboxTemplateList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *SandboxTemplateList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *SandboxTemplateList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *SandboxTemplateList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *SandboxTemplateList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *SandboxTemplateList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *SandboxTemplateList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *SandboxTemplateList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *SandboxTemplateList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *SandboxTemplateList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *SandboxTemplateList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *SandboxTemplateList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *SandboxTemplateList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *SandboxTemplateList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *SandboxTemplateList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *SandboxTemplateList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *SandboxTemplateList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *SandboxTemplateList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *SandboxTemplateList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *SandboxTemplateList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *SandboxTemplateList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *SandboxTemplateList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *SandboxTemplateList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *SandboxTemplateList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *SandboxTemplateList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *SandboxTemplateList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *SandboxTemplateList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *SandboxTemplateList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *SandboxTemplateList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *SandboxTemplateList) GetItems() []SandboxTemplate`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *SandboxTemplateList) GetItemsOk() (*[]SandboxTemplate, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *SandboxTemplateList) SetItems(v []SandboxTemplate)`

SetItems sets Items field to given value.

### HasItems

`func (o *SandboxTemplateList) HasItems() bool`

HasItems returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


