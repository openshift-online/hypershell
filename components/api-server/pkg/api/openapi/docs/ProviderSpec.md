# ProviderSpec

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Category** | **string** |  | 
**Capability** | Pointer to **string** | JSON-encoded capability definition (gateway-agnostic, category-discriminated) | [optional] 
**Profile** | Pointer to **string** | JSON-encoded opaque profile passed through to the gateway provider registration step | [optional] 
**Status** | Pointer to **string** |  | [optional] [readonly] 

## Methods

### NewProviderSpec

`func NewProviderSpec(name string, category string, ) *ProviderSpec`

NewProviderSpec instantiates a new ProviderSpec object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewProviderSpecWithDefaults

`func NewProviderSpecWithDefaults() *ProviderSpec`

NewProviderSpecWithDefaults instantiates a new ProviderSpec object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *ProviderSpec) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ProviderSpec) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ProviderSpec) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ProviderSpec) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *ProviderSpec) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ProviderSpec) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ProviderSpec) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *ProviderSpec) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *ProviderSpec) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ProviderSpec) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ProviderSpec) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *ProviderSpec) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *ProviderSpec) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *ProviderSpec) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *ProviderSpec) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *ProviderSpec) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *ProviderSpec) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *ProviderSpec) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *ProviderSpec) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *ProviderSpec) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *ProviderSpec) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ProviderSpec) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ProviderSpec) SetName(v string)`

SetName sets Name field to given value.


### GetCategory

`func (o *ProviderSpec) GetCategory() string`

GetCategory returns the Category field if non-nil, zero value otherwise.

### GetCategoryOk

`func (o *ProviderSpec) GetCategoryOk() (*string, bool)`

GetCategoryOk returns a tuple with the Category field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCategory

`func (o *ProviderSpec) SetCategory(v string)`

SetCategory sets Category field to given value.


### GetCapability

`func (o *ProviderSpec) GetCapability() string`

GetCapability returns the Capability field if non-nil, zero value otherwise.

### GetCapabilityOk

`func (o *ProviderSpec) GetCapabilityOk() (*string, bool)`

GetCapabilityOk returns a tuple with the Capability field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCapability

`func (o *ProviderSpec) SetCapability(v string)`

SetCapability sets Capability field to given value.

### HasCapability

`func (o *ProviderSpec) HasCapability() bool`

HasCapability returns a boolean if a field has been set.

### GetProfile

`func (o *ProviderSpec) GetProfile() string`

GetProfile returns the Profile field if non-nil, zero value otherwise.

### GetProfileOk

`func (o *ProviderSpec) GetProfileOk() (*string, bool)`

GetProfileOk returns a tuple with the Profile field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProfile

`func (o *ProviderSpec) SetProfile(v string)`

SetProfile sets Profile field to given value.

### HasProfile

`func (o *ProviderSpec) HasProfile() bool`

HasProfile returns a boolean if a field has been set.

### GetStatus

`func (o *ProviderSpec) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ProviderSpec) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ProviderSpec) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *ProviderSpec) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


