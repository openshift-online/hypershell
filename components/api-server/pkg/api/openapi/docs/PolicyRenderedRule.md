# PolicyRenderedRule

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Recognized** | Pointer to **bool** |  | [optional]
**Summary** | Pointer to **[]string** |  | [optional]
**Bounds** | Pointer to [**[]PolicyBound**](PolicyBound.md) |  | [optional]
**Cedar** | Pointer to **string** |  | [optional]

## Methods

### NewPolicyRenderedRule

`func NewPolicyRenderedRule() *PolicyRenderedRule`

NewPolicyRenderedRule instantiates a new PolicyRenderedRule object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicyRenderedRuleWithDefaults

`func NewPolicyRenderedRuleWithDefaults() *PolicyRenderedRule`

NewPolicyRenderedRuleWithDefaults instantiates a new PolicyRenderedRule object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetRecognized

`func (o *PolicyRenderedRule) GetRecognized() bool`

GetRecognized returns the Recognized field if non-nil, zero value otherwise.

### GetRecognizedOk

`func (o *PolicyRenderedRule) GetRecognizedOk() (*bool, bool)`

GetRecognizedOk returns a tuple with the Recognized field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRecognized

`func (o *PolicyRenderedRule) SetRecognized(v bool)`

SetRecognized sets Recognized field to given value.

### HasRecognized

`func (o *PolicyRenderedRule) HasRecognized() bool`

HasRecognized returns a boolean if a field has been set.

### GetSummary

`func (o *PolicyRenderedRule) GetSummary() []string`

GetSummary returns the Summary field if non-nil, zero value otherwise.

### GetSummaryOk

`func (o *PolicyRenderedRule) GetSummaryOk() (*[]string, bool)`

GetSummaryOk returns a tuple with the Summary field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSummary

`func (o *PolicyRenderedRule) SetSummary(v []string)`

SetSummary sets Summary field to given value.

### HasSummary

`func (o *PolicyRenderedRule) HasSummary() bool`

HasSummary returns a boolean if a field has been set.

### GetBounds

`func (o *PolicyRenderedRule) GetBounds() []PolicyBound`

GetBounds returns the Bounds field if non-nil, zero value otherwise.

### GetBoundsOk

`func (o *PolicyRenderedRule) GetBoundsOk() (*[]PolicyBound, bool)`

GetBoundsOk returns a tuple with the Bounds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBounds

`func (o *PolicyRenderedRule) SetBounds(v []PolicyBound)`

SetBounds sets Bounds field to given value.

### HasBounds

`func (o *PolicyRenderedRule) HasBounds() bool`

HasBounds returns a boolean if a field has been set.

### GetCedar

`func (o *PolicyRenderedRule) GetCedar() string`

GetCedar returns the Cedar field if non-nil, zero value otherwise.

### GetCedarOk

`func (o *PolicyRenderedRule) GetCedarOk() (*string, bool)`

GetCedarOk returns a tuple with the Cedar field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCedar

`func (o *PolicyRenderedRule) SetCedar(v string)`

SetCedar sets Cedar field to given value.

### HasCedar

`func (o *PolicyRenderedRule) HasCedar() bool`

HasCedar returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
