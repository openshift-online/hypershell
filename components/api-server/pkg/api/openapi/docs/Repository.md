# Repository

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** | Human-readable identifier for the repository registration | 
**Url** | **string** | Git remote URL (e.g. https://github.com/openshift-online/hypershell) | 
**Provider** | **string** | Provider type the credential targets: github (extensible) | 
**SecretSourceId** | **string** | References a SecretSource holding the provider credential | 
**DefaultBranch** | Pointer to **string** | Branch the import process reads when none is otherwise specified | [optional] 
**ImportPath** | Pointer to **string** | Directory within the repository to import (default .hypershell) | [optional] 
**Status** | Pointer to **string** | Registration/import health | [optional] [readonly] 

## Methods

### NewRepository

`func NewRepository(name string, url string, provider string, secretSourceId string, ) *Repository`

NewRepository instantiates a new Repository object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRepositoryWithDefaults

`func NewRepositoryWithDefaults() *Repository`

NewRepositoryWithDefaults instantiates a new Repository object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *Repository) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *Repository) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *Repository) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *Repository) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *Repository) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *Repository) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *Repository) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *Repository) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *Repository) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *Repository) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *Repository) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *Repository) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *Repository) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *Repository) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *Repository) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *Repository) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *Repository) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *Repository) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *Repository) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *Repository) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *Repository) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *Repository) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *Repository) SetName(v string)`

SetName sets Name field to given value.


### GetUrl

`func (o *Repository) GetUrl() string`

GetUrl returns the Url field if non-nil, zero value otherwise.

### GetUrlOk

`func (o *Repository) GetUrlOk() (*string, bool)`

GetUrlOk returns a tuple with the Url field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUrl

`func (o *Repository) SetUrl(v string)`

SetUrl sets Url field to given value.


### GetProvider

`func (o *Repository) GetProvider() string`

GetProvider returns the Provider field if non-nil, zero value otherwise.

### GetProviderOk

`func (o *Repository) GetProviderOk() (*string, bool)`

GetProviderOk returns a tuple with the Provider field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProvider

`func (o *Repository) SetProvider(v string)`

SetProvider sets Provider field to given value.


### GetSecretSourceId

`func (o *Repository) GetSecretSourceId() string`

GetSecretSourceId returns the SecretSourceId field if non-nil, zero value otherwise.

### GetSecretSourceIdOk

`func (o *Repository) GetSecretSourceIdOk() (*string, bool)`

GetSecretSourceIdOk returns a tuple with the SecretSourceId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSecretSourceId

`func (o *Repository) SetSecretSourceId(v string)`

SetSecretSourceId sets SecretSourceId field to given value.


### GetDefaultBranch

`func (o *Repository) GetDefaultBranch() string`

GetDefaultBranch returns the DefaultBranch field if non-nil, zero value otherwise.

### GetDefaultBranchOk

`func (o *Repository) GetDefaultBranchOk() (*string, bool)`

GetDefaultBranchOk returns a tuple with the DefaultBranch field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaultBranch

`func (o *Repository) SetDefaultBranch(v string)`

SetDefaultBranch sets DefaultBranch field to given value.

### HasDefaultBranch

`func (o *Repository) HasDefaultBranch() bool`

HasDefaultBranch returns a boolean if a field has been set.

### GetImportPath

`func (o *Repository) GetImportPath() string`

GetImportPath returns the ImportPath field if non-nil, zero value otherwise.

### GetImportPathOk

`func (o *Repository) GetImportPathOk() (*string, bool)`

GetImportPathOk returns a tuple with the ImportPath field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImportPath

`func (o *Repository) SetImportPath(v string)`

SetImportPath sets ImportPath field to given value.

### HasImportPath

`func (o *Repository) HasImportPath() bool`

HasImportPath returns a boolean if a field has been set.

### GetStatus

`func (o *Repository) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *Repository) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *Repository) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *Repository) HasStatus() bool`

HasStatus returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


