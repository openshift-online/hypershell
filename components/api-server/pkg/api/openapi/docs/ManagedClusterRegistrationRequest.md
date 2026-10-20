# ManagedClusterRegistrationRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Human-readable spoke name, unique per platform (e.g. hyp0-mc1). Must match on every subsequent call. |
**Capabilities** | Pointer to **[]string** | Complete self-declared capability set. Optional for backward compatibility: a control plane that predates the capability model may omit it, in which case the heartbeat is accepted as a no-op that preserves any stored capabilities. | [optional]

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


### GetCapabilities

`func (o *ManagedClusterRegistrationRequest) GetCapabilities() []string`

GetCapabilities returns the Capabilities field if non-nil, zero value otherwise.

### GetCapabilitiesOk

`func (o *ManagedClusterRegistrationRequest) GetCapabilitiesOk() (*[]string, bool)`

GetCapabilitiesOk returns a tuple with the Capabilities field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCapabilities

`func (o *ManagedClusterRegistrationRequest) SetCapabilities(v []string)`

SetCapabilities sets Capabilities field to given value.

### HasCapabilities

`func (o *ManagedClusterRegistrationRequest) HasCapabilities() bool`

HasCapabilities returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
