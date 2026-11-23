# PolicyAuditFindingList

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
**Items** | Pointer to [**[]PolicyAuditFinding**](PolicyAuditFinding.md) |  | [optional]

## Methods

### NewPolicyAuditFindingList

`func NewPolicyAuditFindingList() *PolicyAuditFindingList`

NewPolicyAuditFindingList instantiates a new PolicyAuditFindingList object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicyAuditFindingListWithDefaults

`func NewPolicyAuditFindingListWithDefaults() *PolicyAuditFindingList`

NewPolicyAuditFindingListWithDefaults instantiates a new PolicyAuditFindingList object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *PolicyAuditFindingList) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *PolicyAuditFindingList) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *PolicyAuditFindingList) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *PolicyAuditFindingList) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetPage

`func (o *PolicyAuditFindingList) GetPage() int32`

GetPage returns the Page field if non-nil, zero value otherwise.

### GetPageOk

`func (o *PolicyAuditFindingList) GetPageOk() (*int32, bool)`

GetPageOk returns a tuple with the Page field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPage

`func (o *PolicyAuditFindingList) SetPage(v int32)`

SetPage sets Page field to given value.

### HasPage

`func (o *PolicyAuditFindingList) HasPage() bool`

HasPage returns a boolean if a field has been set.

### GetSize

`func (o *PolicyAuditFindingList) GetSize() int32`

GetSize returns the Size field if non-nil, zero value otherwise.

### GetSizeOk

`func (o *PolicyAuditFindingList) GetSizeOk() (*int32, bool)`

GetSizeOk returns a tuple with the Size field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSize

`func (o *PolicyAuditFindingList) SetSize(v int32)`

SetSize sets Size field to given value.

### HasSize

`func (o *PolicyAuditFindingList) HasSize() bool`

HasSize returns a boolean if a field has been set.

### GetTotal

`func (o *PolicyAuditFindingList) GetTotal() int32`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *PolicyAuditFindingList) GetTotalOk() (*int32, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *PolicyAuditFindingList) SetTotal(v int32)`

SetTotal sets Total field to given value.

### HasTotal

`func (o *PolicyAuditFindingList) HasTotal() bool`

HasTotal returns a boolean if a field has been set.

### GetId

`func (o *PolicyAuditFindingList) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *PolicyAuditFindingList) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *PolicyAuditFindingList) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *PolicyAuditFindingList) HasId() bool`

HasId returns a boolean if a field has been set.

### GetHref

`func (o *PolicyAuditFindingList) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *PolicyAuditFindingList) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *PolicyAuditFindingList) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *PolicyAuditFindingList) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *PolicyAuditFindingList) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *PolicyAuditFindingList) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *PolicyAuditFindingList) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *PolicyAuditFindingList) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *PolicyAuditFindingList) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *PolicyAuditFindingList) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *PolicyAuditFindingList) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *PolicyAuditFindingList) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetItems

`func (o *PolicyAuditFindingList) GetItems() []PolicyAuditFinding`

GetItems returns the Items field if non-nil, zero value otherwise.

### GetItemsOk

`func (o *PolicyAuditFindingList) GetItemsOk() (*[]PolicyAuditFinding, bool)`

GetItemsOk returns a tuple with the Items field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetItems

`func (o *PolicyAuditFindingList) SetItems(v []PolicyAuditFinding)`

SetItems sets Items field to given value.

### HasItems

`func (o *PolicyAuditFindingList) HasItems() bool`

HasItems returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
