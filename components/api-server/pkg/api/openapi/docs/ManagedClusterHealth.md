# ManagedClusterHealth

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Overall** | **string** | Rollup of the per-check conditions. |
**ObservedAt** | Pointer to **time.Time** | When the last self-check run completed. | [optional]
**DurationMs** | Pointer to **int64** | Duration of the last self-check run in milliseconds. | [optional]
**Conditions** | [**[]ManagedClusterHealthCondition**](ManagedClusterHealthCondition.md) | One condition per check in the bounded check catalog. |

## Methods

### NewManagedClusterHealth

`func NewManagedClusterHealth(overall string, conditions []ManagedClusterHealthCondition, ) *ManagedClusterHealth`

NewManagedClusterHealth instantiates a new ManagedClusterHealth object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterHealthWithDefaults

`func NewManagedClusterHealthWithDefaults() *ManagedClusterHealth`

NewManagedClusterHealthWithDefaults instantiates a new ManagedClusterHealth object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetOverall

`func (o *ManagedClusterHealth) GetOverall() string`

GetOverall returns the Overall field if non-nil, zero value otherwise.

### GetOverallOk

`func (o *ManagedClusterHealth) GetOverallOk() (*string, bool)`

GetOverallOk returns a tuple with the Overall field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOverall

`func (o *ManagedClusterHealth) SetOverall(v string)`

SetOverall sets Overall field to given value.


### GetObservedAt

`func (o *ManagedClusterHealth) GetObservedAt() time.Time`

GetObservedAt returns the ObservedAt field if non-nil, zero value otherwise.

### GetObservedAtOk

`func (o *ManagedClusterHealth) GetObservedAtOk() (*time.Time, bool)`

GetObservedAtOk returns a tuple with the ObservedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedAt

`func (o *ManagedClusterHealth) SetObservedAt(v time.Time)`

SetObservedAt sets ObservedAt field to given value.

### HasObservedAt

`func (o *ManagedClusterHealth) HasObservedAt() bool`

HasObservedAt returns a boolean if a field has been set.

### GetDurationMs

`func (o *ManagedClusterHealth) GetDurationMs() int64`

GetDurationMs returns the DurationMs field if non-nil, zero value otherwise.

### GetDurationMsOk

`func (o *ManagedClusterHealth) GetDurationMsOk() (*int64, bool)`

GetDurationMsOk returns a tuple with the DurationMs field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDurationMs

`func (o *ManagedClusterHealth) SetDurationMs(v int64)`

SetDurationMs sets DurationMs field to given value.

### HasDurationMs

`func (o *ManagedClusterHealth) HasDurationMs() bool`

HasDurationMs returns a boolean if a field has been set.

### GetConditions

`func (o *ManagedClusterHealth) GetConditions() []ManagedClusterHealthCondition`

GetConditions returns the Conditions field if non-nil, zero value otherwise.

### GetConditionsOk

`func (o *ManagedClusterHealth) GetConditionsOk() (*[]ManagedClusterHealthCondition, bool)`

GetConditionsOk returns a tuple with the Conditions field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConditions

`func (o *ManagedClusterHealth) SetConditions(v []ManagedClusterHealthCondition)`

SetConditions sets Conditions field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
