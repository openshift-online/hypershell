# SandboxTemplate

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**Image** | **string** | OCI image for agent worker sandboxes | 
**NamePrefix** | Pointer to **string** | Prefix applied to sandbox pod names | [optional] 
**Policy** | Pointer to **string** | JSON-encoded filesystem, network, and process policy for sandboxes | [optional] 
**Status** | Pointer to **string** |  | [optional] [readonly] 

## Methods

### NewSandboxTemplate

`func NewSandboxTemplate(name string, image string, ) *SandboxTemplate`

NewSandboxTemplate instantiates a new SandboxTemplate object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSandboxTemplateWithDefaults

`func NewSandboxTemplateWithDefaults() *SandboxTemplate`

NewSandboxTemplateWithDefaults instantiates a new SandboxTemplate object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *SandboxTemplate) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *SandboxTemplate) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *SandboxTemplate) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *SandboxTemplate) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *SandboxTemplate) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *SandboxTemplate) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *SandboxTemplate) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *SandboxTemplate) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *SandboxTemplate) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *SandboxTemplate) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *SandboxTemplate) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *SandboxTemplate) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *SandboxTemplate) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *SandboxTemplate) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *SandboxTemplate) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *SandboxTemplate) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *SandboxTemplate) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *SandboxTemplate) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *SandboxTemplate) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *SandboxTemplate) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *SandboxTemplate) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *SandboxTemplate) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *SandboxTemplate) SetName(v string)`

SetName sets Name field to given value.


### GetImage

`func (o *SandboxTemplate) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *SandboxTemplate) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *SandboxTemplate) SetImage(v string)`

SetImage sets Image field to given value.


### GetNamePrefix

`func (o *SandboxTemplate) GetNamePrefix() string`

GetNamePrefix returns the NamePrefix field if non-nil, zero value otherwise.

### GetNamePrefixOk

`func (o *SandboxTemplate) GetNamePrefixOk() (*string, bool)`

GetNamePrefixOk returns a tuple with the NamePrefix field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNamePrefix

`func (o *SandboxTemplate) SetNamePrefix(v string)`

SetNamePrefix sets NamePrefix field to given value.

### HasNamePrefix

`func (o *SandboxTemplate) HasNamePrefix() bool`

HasNamePrefix returns a boolean if a field has been set.

### GetPolicy

`func (o *SandboxTemplate) GetPolicy() string`

GetPolicy returns the Policy field if non-nil, zero value otherwise.

### GetPolicyOk

`func (o *SandboxTemplate) GetPolicyOk() (*string, bool)`

GetPolicyOk returns a tuple with the Policy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPolicy

`func (o *SandboxTemplate) SetPolicy(v string)`

SetPolicy sets Policy field to given value.

### HasPolicy

`func (o *SandboxTemplate) HasPolicy() bool`

HasPolicy returns a boolean if a field has been set.

### GetStatus

`func (o *SandboxTemplate) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *SandboxTemplate) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *SandboxTemplate) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *SandboxTemplate) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


