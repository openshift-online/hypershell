# ManagedClusterRegistrationRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Human-readable spoke name, unique per platform (e.g. hyp0-mc1). Must match on every subsequent call. |
**Capabilities** | **[]string** | Complete self-declared capability set, sent on every registration and heartbeat. An omitted, null, or empty set is rejected with 400. |
**Health** | Pointer to [**ManagedClusterHealth**](ManagedClusterHealth.md) | Optional on-cluster self-check verdict (PLAT-CLUS-16). When present it is stored on the ManagedCluster in the same operation that updates last_seen_at. Omitting it (absent or null) preserves the stored verdict; a malformed verdict is rejected with 400 without changing the stored verdict or last_seen_at. | [optional]

## Methods

### NewManagedClusterRegistrationRequest

`func NewManagedClusterRegistrationRequest(name string, capabilities []string, ) *ManagedClusterRegistrationRequest`

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


### GetHealth

`func (o *ManagedClusterRegistrationRequest) GetHealth() ManagedClusterHealth`

GetHealth returns the Health field if non-nil, zero value otherwise.

### GetHealthOk

`func (o *ManagedClusterRegistrationRequest) GetHealthOk() (*ManagedClusterHealth, bool)`

GetHealthOk returns a tuple with the Health field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHealth

`func (o *ManagedClusterRegistrationRequest) SetHealth(v ManagedClusterHealth)`

SetHealth sets Health field to given value.

### HasHealth

`func (o *ManagedClusterRegistrationRequest) HasHealth() bool`

HasHealth returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
