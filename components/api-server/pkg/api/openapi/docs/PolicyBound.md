# PolicyBound

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Field** | Pointer to **string** | The Cedar context attribute, for example cpuRequestMillis | [optional]
**Label** | Pointer to **string** |  | [optional]
**Op** | Pointer to **string** | The forbid comparison operator | [optional]
**Value** | Pointer to **int64** | The bound in schema units (millicores, bytes) | [optional]
**Display** | Pointer to **string** | The bound as a quantity, for example 4 or 16Gi | [optional]
**Text** | Pointer to **string** | The plain-words bound, for example \&quot;CPU request at most 4\&quot; | [optional]

## Methods

### NewPolicyBound

`func NewPolicyBound() *PolicyBound`

NewPolicyBound instantiates a new PolicyBound object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicyBoundWithDefaults

`func NewPolicyBoundWithDefaults() *PolicyBound`

NewPolicyBoundWithDefaults instantiates a new PolicyBound object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetField

`func (o *PolicyBound) GetField() string`

GetField returns the Field field if non-nil, zero value otherwise.

### GetFieldOk

`func (o *PolicyBound) GetFieldOk() (*string, bool)`

GetFieldOk returns a tuple with the Field field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetField

`func (o *PolicyBound) SetField(v string)`

SetField sets Field field to given value.

### HasField

`func (o *PolicyBound) HasField() bool`

HasField returns a boolean if a field has been set.

### GetLabel

`func (o *PolicyBound) GetLabel() string`

GetLabel returns the Label field if non-nil, zero value otherwise.

### GetLabelOk

`func (o *PolicyBound) GetLabelOk() (*string, bool)`

GetLabelOk returns a tuple with the Label field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLabel

`func (o *PolicyBound) SetLabel(v string)`

SetLabel sets Label field to given value.

### HasLabel

`func (o *PolicyBound) HasLabel() bool`

HasLabel returns a boolean if a field has been set.

### GetOp

`func (o *PolicyBound) GetOp() string`

GetOp returns the Op field if non-nil, zero value otherwise.

### GetOpOk

`func (o *PolicyBound) GetOpOk() (*string, bool)`

GetOpOk returns a tuple with the Op field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOp

`func (o *PolicyBound) SetOp(v string)`

SetOp sets Op field to given value.

### HasOp

`func (o *PolicyBound) HasOp() bool`

HasOp returns a boolean if a field has been set.

### GetValue

`func (o *PolicyBound) GetValue() int64`

GetValue returns the Value field if non-nil, zero value otherwise.

### GetValueOk

`func (o *PolicyBound) GetValueOk() (*int64, bool)`

GetValueOk returns a tuple with the Value field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValue

`func (o *PolicyBound) SetValue(v int64)`

SetValue sets Value field to given value.

### HasValue

`func (o *PolicyBound) HasValue() bool`

HasValue returns a boolean if a field has been set.

### GetDisplay

`func (o *PolicyBound) GetDisplay() string`

GetDisplay returns the Display field if non-nil, zero value otherwise.

### GetDisplayOk

`func (o *PolicyBound) GetDisplayOk() (*string, bool)`

GetDisplayOk returns a tuple with the Display field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDisplay

`func (o *PolicyBound) SetDisplay(v string)`

SetDisplay sets Display field to given value.

### HasDisplay

`func (o *PolicyBound) HasDisplay() bool`

HasDisplay returns a boolean if a field has been set.

### GetText

`func (o *PolicyBound) GetText() string`

GetText returns the Text field if non-nil, zero value otherwise.

### GetTextOk

`func (o *PolicyBound) GetTextOk() (*string, bool)`

GetTextOk returns a tuple with the Text field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetText

`func (o *PolicyBound) SetText(v string)`

SetText sets Text field to given value.

### HasText

`func (o *PolicyBound) HasText() bool`

HasText returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
