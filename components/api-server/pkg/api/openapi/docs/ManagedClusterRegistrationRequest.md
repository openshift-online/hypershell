# ManagedClusterRegistrationRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Human-readable spoke name, unique per fleet (e.g. hyp0-mc1). Must match on every subsequent call. | 
**Description** | Pointer to **string** | Optional description of the spoke. | [optional] 

## Methods

### NewManagedClusterRegistrationRequest

`func NewManagedClusterRegistrationRequest(name string, ) *ManagedClusterRegistrationRequest`

NewManagedClusterRegistrationRequest instantiates a new ManagedClusterRegistrationRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterRegistrationRequestWithDefaults

`func NewManagedClusterRegistrationRequestWithDefaults() *ManagedClusterRegistrationRequest`

NewManagedClusterRegistrationRequestWithDefaults instantiates a new ManagedClusterRegistrationRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *ManagedClusterRegistrationRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ManagedClusterRegistrationRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ManagedClusterRegistrationRequest) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *ManagedClusterRegistrationRequest) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *ManagedClusterRegistrationRequest) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *ManagedClusterRegistrationRequest) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *ManagedClusterRegistrationRequest) HasDescription() bool`

HasDescription returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


