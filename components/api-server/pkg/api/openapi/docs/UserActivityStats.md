# UserActivityStats

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**TotalRegistered** | **int64** |  | 
**RegisteredLast7Days** | **int64** |  | 
**RegisteredLast30Days** | **int64** |  | 
**ActiveLast7Days** | **int64** |  | 
**ActiveLast30Days** | **int64** |  | 
**RegistrationDaily** | [**[]UserDailyCount**](UserDailyCount.md) | New registrations per UTC day for the last 30 calendar days (inclusive) | 
**ActiveDaily** | [**[]UserDailyCount**](UserDailyCount.md) | Distinct active users per UTC day for the last 30 calendar days (inclusive) | 

## Methods

### NewUserActivityStats

`func NewUserActivityStats(totalRegistered int64, registeredLast7Days int64, registeredLast30Days int64, activeLast7Days int64, activeLast30Days int64, registrationDaily []UserDailyCount, activeDaily []UserDailyCount, ) *UserActivityStats`

NewUserActivityStats instantiates a new UserActivityStats object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewUserActivityStatsWithDefaults

`func NewUserActivityStatsWithDefaults() *UserActivityStats`

NewUserActivityStatsWithDefaults instantiates a new UserActivityStats object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetTotalRegistered

`func (o *UserActivityStats) GetTotalRegistered() int64`

GetTotalRegistered returns the TotalRegistered field if non-nil, zero value otherwise.

### GetTotalRegisteredOk

`func (o *UserActivityStats) GetTotalRegisteredOk() (*int64, bool)`

GetTotalRegisteredOk returns a tuple with the TotalRegistered field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotalRegistered

`func (o *UserActivityStats) SetTotalRegistered(v int64)`

SetTotalRegistered sets TotalRegistered field to given value.


### GetRegisteredLast7Days

`func (o *UserActivityStats) GetRegisteredLast7Days() int64`

GetRegisteredLast7Days returns the RegisteredLast7Days field if non-nil, zero value otherwise.

### GetRegisteredLast7DaysOk

`func (o *UserActivityStats) GetRegisteredLast7DaysOk() (*int64, bool)`

GetRegisteredLast7DaysOk returns a tuple with the RegisteredLast7Days field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegisteredLast7Days

`func (o *UserActivityStats) SetRegisteredLast7Days(v int64)`

SetRegisteredLast7Days sets RegisteredLast7Days field to given value.


### GetRegisteredLast30Days

`func (o *UserActivityStats) GetRegisteredLast30Days() int64`

GetRegisteredLast30Days returns the RegisteredLast30Days field if non-nil, zero value otherwise.

### GetRegisteredLast30DaysOk

`func (o *UserActivityStats) GetRegisteredLast30DaysOk() (*int64, bool)`

GetRegisteredLast30DaysOk returns a tuple with the RegisteredLast30Days field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegisteredLast30Days

`func (o *UserActivityStats) SetRegisteredLast30Days(v int64)`

SetRegisteredLast30Days sets RegisteredLast30Days field to given value.


### GetActiveLast7Days

`func (o *UserActivityStats) GetActiveLast7Days() int64`

GetActiveLast7Days returns the ActiveLast7Days field if non-nil, zero value otherwise.

### GetActiveLast7DaysOk

`func (o *UserActivityStats) GetActiveLast7DaysOk() (*int64, bool)`

GetActiveLast7DaysOk returns a tuple with the ActiveLast7Days field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetActiveLast7Days

`func (o *UserActivityStats) SetActiveLast7Days(v int64)`

SetActiveLast7Days sets ActiveLast7Days field to given value.


### GetActiveLast30Days

`func (o *UserActivityStats) GetActiveLast30Days() int64`

GetActiveLast30Days returns the ActiveLast30Days field if non-nil, zero value otherwise.

### GetActiveLast30DaysOk

`func (o *UserActivityStats) GetActiveLast30DaysOk() (*int64, bool)`

GetActiveLast30DaysOk returns a tuple with the ActiveLast30Days field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetActiveLast30Days

`func (o *UserActivityStats) SetActiveLast30Days(v int64)`

SetActiveLast30Days sets ActiveLast30Days field to given value.


### GetRegistrationDaily

`func (o *UserActivityStats) GetRegistrationDaily() []UserDailyCount`

GetRegistrationDaily returns the RegistrationDaily field if non-nil, zero value otherwise.

### GetRegistrationDailyOk

`func (o *UserActivityStats) GetRegistrationDailyOk() (*[]UserDailyCount, bool)`

GetRegistrationDailyOk returns a tuple with the RegistrationDaily field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegistrationDaily

`func (o *UserActivityStats) SetRegistrationDaily(v []UserDailyCount)`

SetRegistrationDaily sets RegistrationDaily field to given value.


### GetActiveDaily

`func (o *UserActivityStats) GetActiveDaily() []UserDailyCount`

GetActiveDaily returns the ActiveDaily field if non-nil, zero value otherwise.

### GetActiveDailyOk

`func (o *UserActivityStats) GetActiveDailyOk() (*[]UserDailyCount, bool)`

GetActiveDailyOk returns a tuple with the ActiveDaily field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetActiveDaily

`func (o *UserActivityStats) SetActiveDaily(v []UserDailyCount)`

SetActiveDaily sets ActiveDaily field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


