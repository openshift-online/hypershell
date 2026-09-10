# ManagedClusterRegistrationResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ClusterId** | **string** | Stable KSUID assigned to this managed cluster. Use as the cluster filter for WatchGateways. | 

## Methods

### NewManagedClusterRegistrationResponse

`func NewManagedClusterRegistrationResponse(clusterId string, ) *ManagedClusterRegistrationResponse`

NewManagedClusterRegistrationResponse instantiates a new ManagedClusterRegistrationResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterRegistrationResponseWithDefaults

`func NewManagedClusterRegistrationResponseWithDefaults() *ManagedClusterRegistrationResponse`

NewManagedClusterRegistrationResponseWithDefaults instantiates a new ManagedClusterRegistrationResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetClusterId

`func (o *ManagedClusterRegistrationResponse) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *ManagedClusterRegistrationResponse) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *ManagedClusterRegistrationResponse) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


