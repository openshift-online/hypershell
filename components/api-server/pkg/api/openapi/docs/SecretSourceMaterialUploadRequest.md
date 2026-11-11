# SecretSourceMaterialUploadRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Value** | **string** | The durable secret material to store. Write-only (one-way) - it is written to the configured secret manager at the secret source&#39;s server-derived convention path and never persisted in the database, logged, or returned on any read or upload response. |

## Methods

### NewSecretSourceMaterialUploadRequest

`func NewSecretSourceMaterialUploadRequest(value string, ) *SecretSourceMaterialUploadRequest`

NewSecretSourceMaterialUploadRequest instantiates a new SecretSourceMaterialUploadRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSecretSourceMaterialUploadRequestWithDefaults

`func NewSecretSourceMaterialUploadRequestWithDefaults() *SecretSourceMaterialUploadRequest`

NewSecretSourceMaterialUploadRequestWithDefaults instantiates a new SecretSourceMaterialUploadRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetValue

`func (o *SecretSourceMaterialUploadRequest) GetValue() string`

GetValue returns the Value field if non-nil, zero value otherwise.

### GetValueOk

`func (o *SecretSourceMaterialUploadRequest) GetValueOk() (*string, bool)`

GetValueOk returns a tuple with the Value field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValue

`func (o *SecretSourceMaterialUploadRequest) SetValue(v string)`

SetValue sets Value field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
