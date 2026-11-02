# CITrustAllOfStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ExchangeEndpoint** | Pointer to **string** | The cluster Keycloak token endpoint for token exchange | [optional]
**Workspace** | Pointer to **string** | The derived forge-namespaced workspace (e.g. gh-myorg, gl-mygroup) | [optional]
**WorkflowHint** | Pointer to **string** | A runnable CI snippet the tenant pastes to mint and exchange the token | [optional]

## Methods

### NewCITrustAllOfStatus

`func NewCITrustAllOfStatus() *CITrustAllOfStatus`

NewCITrustAllOfStatus instantiates a new CITrustAllOfStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCITrustAllOfStatusWithDefaults

`func NewCITrustAllOfStatusWithDefaults() *CITrustAllOfStatus`

NewCITrustAllOfStatusWithDefaults instantiates a new CITrustAllOfStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetExchangeEndpoint

`func (o *CITrustAllOfStatus) GetExchangeEndpoint() string`

GetExchangeEndpoint returns the ExchangeEndpoint field if non-nil, zero value otherwise.

### GetExchangeEndpointOk

`func (o *CITrustAllOfStatus) GetExchangeEndpointOk() (*string, bool)`

GetExchangeEndpointOk returns a tuple with the ExchangeEndpoint field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExchangeEndpoint

`func (o *CITrustAllOfStatus) SetExchangeEndpoint(v string)`

SetExchangeEndpoint sets ExchangeEndpoint field to given value.

### HasExchangeEndpoint

`func (o *CITrustAllOfStatus) HasExchangeEndpoint() bool`

HasExchangeEndpoint returns a boolean if a field has been set.

### GetWorkspace

`func (o *CITrustAllOfStatus) GetWorkspace() string`

GetWorkspace returns the Workspace field if non-nil, zero value otherwise.

### GetWorkspaceOk

`func (o *CITrustAllOfStatus) GetWorkspaceOk() (*string, bool)`

GetWorkspaceOk returns a tuple with the Workspace field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWorkspace

`func (o *CITrustAllOfStatus) SetWorkspace(v string)`

SetWorkspace sets Workspace field to given value.

### HasWorkspace

`func (o *CITrustAllOfStatus) HasWorkspace() bool`

HasWorkspace returns a boolean if a field has been set.

### GetWorkflowHint

`func (o *CITrustAllOfStatus) GetWorkflowHint() string`

GetWorkflowHint returns the WorkflowHint field if non-nil, zero value otherwise.

### GetWorkflowHintOk

`func (o *CITrustAllOfStatus) GetWorkflowHintOk() (*string, bool)`

GetWorkflowHintOk returns a tuple with the WorkflowHint field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWorkflowHint

`func (o *CITrustAllOfStatus) SetWorkflowHint(v string)`

SetWorkflowHint sets WorkflowHint field to given value.

### HasWorkflowHint

`func (o *CITrustAllOfStatus) HasWorkflowHint() bool`

HasWorkflowHint returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
