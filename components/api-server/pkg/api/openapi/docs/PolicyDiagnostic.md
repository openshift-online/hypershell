# PolicyDiagnostic

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Line** | Pointer to **int32** | 1-based line of the offending statement, 0 when no line applies | [optional]
**Statement** | Pointer to **int32** |  | [optional]
**Message** | Pointer to **string** |  | [optional]
**Suggestion** | Pointer to **string** |  | [optional]

## Methods

### NewPolicyDiagnostic

`func NewPolicyDiagnostic() *PolicyDiagnostic`

NewPolicyDiagnostic instantiates a new PolicyDiagnostic object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicyDiagnosticWithDefaults

`func NewPolicyDiagnosticWithDefaults() *PolicyDiagnostic`

NewPolicyDiagnosticWithDefaults instantiates a new PolicyDiagnostic object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetLine

`func (o *PolicyDiagnostic) GetLine() int32`

GetLine returns the Line field if non-nil, zero value otherwise.

### GetLineOk

`func (o *PolicyDiagnostic) GetLineOk() (*int32, bool)`

GetLineOk returns a tuple with the Line field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLine

`func (o *PolicyDiagnostic) SetLine(v int32)`

SetLine sets Line field to given value.

### HasLine

`func (o *PolicyDiagnostic) HasLine() bool`

HasLine returns a boolean if a field has been set.

### GetStatement

`func (o *PolicyDiagnostic) GetStatement() int32`

GetStatement returns the Statement field if non-nil, zero value otherwise.

### GetStatementOk

`func (o *PolicyDiagnostic) GetStatementOk() (*int32, bool)`

GetStatementOk returns a tuple with the Statement field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatement

`func (o *PolicyDiagnostic) SetStatement(v int32)`

SetStatement sets Statement field to given value.

### HasStatement

`func (o *PolicyDiagnostic) HasStatement() bool`

HasStatement returns a boolean if a field has been set.

### GetMessage

`func (o *PolicyDiagnostic) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *PolicyDiagnostic) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *PolicyDiagnostic) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *PolicyDiagnostic) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetSuggestion

`func (o *PolicyDiagnostic) GetSuggestion() string`

GetSuggestion returns the Suggestion field if non-nil, zero value otherwise.

### GetSuggestionOk

`func (o *PolicyDiagnostic) GetSuggestionOk() (*string, bool)`

GetSuggestionOk returns a tuple with the Suggestion field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSuggestion

`func (o *PolicyDiagnostic) SetSuggestion(v string)`

SetSuggestion sets Suggestion field to given value.

### HasSuggestion

`func (o *PolicyDiagnostic) HasSuggestion() bool`

HasSuggestion returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
