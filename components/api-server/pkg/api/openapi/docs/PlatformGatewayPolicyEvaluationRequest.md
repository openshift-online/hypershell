# PlatformGatewayPolicyEvaluationRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional]
**Cedar** | **string** |  |
**UserMessage** | Pointer to **string** |  | [optional]
**DocumentationUrl** | Pointer to **string** |  | [optional]
**Sample** | Pointer to [**PolicySampleRequest**](PolicySampleRequest.md) |  | [optional]

## Methods

### NewPlatformGatewayPolicyEvaluationRequest

`func NewPlatformGatewayPolicyEvaluationRequest(cedar string, ) *PlatformGatewayPolicyEvaluationRequest`

NewPlatformGatewayPolicyEvaluationRequest instantiates a new PlatformGatewayPolicyEvaluationRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyEvaluationRequestWithDefaults

`func NewPlatformGatewayPolicyEvaluationRequestWithDefaults() *PlatformGatewayPolicyEvaluationRequest`

NewPlatformGatewayPolicyEvaluationRequestWithDefaults instantiates a new PlatformGatewayPolicyEvaluationRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *PlatformGatewayPolicyEvaluationRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *PlatformGatewayPolicyEvaluationRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *PlatformGatewayPolicyEvaluationRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *PlatformGatewayPolicyEvaluationRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetCedar

`func (o *PlatformGatewayPolicyEvaluationRequest) GetCedar() string`

GetCedar returns the Cedar field if non-nil, zero value otherwise.

### GetCedarOk

`func (o *PlatformGatewayPolicyEvaluationRequest) GetCedarOk() (*string, bool)`

GetCedarOk returns a tuple with the Cedar field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCedar

`func (o *PlatformGatewayPolicyEvaluationRequest) SetCedar(v string)`

SetCedar sets Cedar field to given value.


### GetUserMessage

`func (o *PlatformGatewayPolicyEvaluationRequest) GetUserMessage() string`

GetUserMessage returns the UserMessage field if non-nil, zero value otherwise.

### GetUserMessageOk

`func (o *PlatformGatewayPolicyEvaluationRequest) GetUserMessageOk() (*string, bool)`

GetUserMessageOk returns a tuple with the UserMessage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUserMessage

`func (o *PlatformGatewayPolicyEvaluationRequest) SetUserMessage(v string)`

SetUserMessage sets UserMessage field to given value.

### HasUserMessage

`func (o *PlatformGatewayPolicyEvaluationRequest) HasUserMessage() bool`

HasUserMessage returns a boolean if a field has been set.

### GetDocumentationUrl

`func (o *PlatformGatewayPolicyEvaluationRequest) GetDocumentationUrl() string`

GetDocumentationUrl returns the DocumentationUrl field if non-nil, zero value otherwise.

### GetDocumentationUrlOk

`func (o *PlatformGatewayPolicyEvaluationRequest) GetDocumentationUrlOk() (*string, bool)`

GetDocumentationUrlOk returns a tuple with the DocumentationUrl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDocumentationUrl

`func (o *PlatformGatewayPolicyEvaluationRequest) SetDocumentationUrl(v string)`

SetDocumentationUrl sets DocumentationUrl field to given value.

### HasDocumentationUrl

`func (o *PlatformGatewayPolicyEvaluationRequest) HasDocumentationUrl() bool`

HasDocumentationUrl returns a boolean if a field has been set.

### GetSample

`func (o *PlatformGatewayPolicyEvaluationRequest) GetSample() PolicySampleRequest`

GetSample returns the Sample field if non-nil, zero value otherwise.

### GetSampleOk

`func (o *PlatformGatewayPolicyEvaluationRequest) GetSampleOk() (*PolicySampleRequest, bool)`

GetSampleOk returns a tuple with the Sample field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSample

`func (o *PlatformGatewayPolicyEvaluationRequest) SetSample(v PolicySampleRequest)`

SetSample sets Sample field to given value.

### HasSample

`func (o *PlatformGatewayPolicyEvaluationRequest) HasSample() bool`

HasSample returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
