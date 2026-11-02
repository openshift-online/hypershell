# CITrustAllOfSubject

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Repository** | Pointer to **string** | GitHub/Forgejo: owner/name repository identifier | [optional]
**ProjectPath** | Pointer to **string** | GitLab: group/project path identifier | [optional]
**Ref** | Pointer to **string** | Optional git ref constraint (e.g. refs/heads/main) | [optional]
**Environment** | Pointer to **string** | Optional deployment environment constraint | [optional]
**RefProtected** | Pointer to **bool** | GitLab: require a protected ref | [optional]

## Methods

### NewCITrustAllOfSubject

`func NewCITrustAllOfSubject() *CITrustAllOfSubject`

NewCITrustAllOfSubject instantiates a new CITrustAllOfSubject object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCITrustAllOfSubjectWithDefaults

`func NewCITrustAllOfSubjectWithDefaults() *CITrustAllOfSubject`

NewCITrustAllOfSubjectWithDefaults instantiates a new CITrustAllOfSubject object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetRepository

`func (o *CITrustAllOfSubject) GetRepository() string`

GetRepository returns the Repository field if non-nil, zero value otherwise.

### GetRepositoryOk

`func (o *CITrustAllOfSubject) GetRepositoryOk() (*string, bool)`

GetRepositoryOk returns a tuple with the Repository field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRepository

`func (o *CITrustAllOfSubject) SetRepository(v string)`

SetRepository sets Repository field to given value.

### HasRepository

`func (o *CITrustAllOfSubject) HasRepository() bool`

HasRepository returns a boolean if a field has been set.

### GetProjectPath

`func (o *CITrustAllOfSubject) GetProjectPath() string`

GetProjectPath returns the ProjectPath field if non-nil, zero value otherwise.

### GetProjectPathOk

`func (o *CITrustAllOfSubject) GetProjectPathOk() (*string, bool)`

GetProjectPathOk returns a tuple with the ProjectPath field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProjectPath

`func (o *CITrustAllOfSubject) SetProjectPath(v string)`

SetProjectPath sets ProjectPath field to given value.

### HasProjectPath

`func (o *CITrustAllOfSubject) HasProjectPath() bool`

HasProjectPath returns a boolean if a field has been set.

### GetRef

`func (o *CITrustAllOfSubject) GetRef() string`

GetRef returns the Ref field if non-nil, zero value otherwise.

### GetRefOk

`func (o *CITrustAllOfSubject) GetRefOk() (*string, bool)`

GetRefOk returns a tuple with the Ref field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRef

`func (o *CITrustAllOfSubject) SetRef(v string)`

SetRef sets Ref field to given value.

### HasRef

`func (o *CITrustAllOfSubject) HasRef() bool`

HasRef returns a boolean if a field has been set.

### GetEnvironment

`func (o *CITrustAllOfSubject) GetEnvironment() string`

GetEnvironment returns the Environment field if non-nil, zero value otherwise.

### GetEnvironmentOk

`func (o *CITrustAllOfSubject) GetEnvironmentOk() (*string, bool)`

GetEnvironmentOk returns a tuple with the Environment field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEnvironment

`func (o *CITrustAllOfSubject) SetEnvironment(v string)`

SetEnvironment sets Environment field to given value.

### HasEnvironment

`func (o *CITrustAllOfSubject) HasEnvironment() bool`

HasEnvironment returns a boolean if a field has been set.

### GetRefProtected

`func (o *CITrustAllOfSubject) GetRefProtected() bool`

GetRefProtected returns the RefProtected field if non-nil, zero value otherwise.

### GetRefProtectedOk

`func (o *CITrustAllOfSubject) GetRefProtectedOk() (*bool, bool)`

GetRefProtectedOk returns a tuple with the RefProtected field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRefProtected

`func (o *CITrustAllOfSubject) SetRefProtected(v bool)`

SetRefProtected sets RefProtected field to given value.

### HasRefProtected

`func (o *CITrustAllOfSubject) HasRefProtected() bool`

HasRefProtected returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
