# PolicyAuditFinding

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional]
**OccurredAt** | Pointer to **time.Time** |  | [optional]
**GatewayId** | Pointer to **string** |  | [optional]
**PolicyNames** | Pointer to **[]string** |  | [optional]
**BundleRevision** | Pointer to **string** |  | [optional]
**PrincipalSubject** | Pointer to **string** |  | [optional]
**Operation** | Pointer to **string** |  | [optional]
**Outcome** | Pointer to **string** |  | [optional]
**RequestSummary** | Pointer to **map[string]int64** |  | [optional]

## Methods

### NewPolicyAuditFinding

`func NewPolicyAuditFinding() *PolicyAuditFinding`

NewPolicyAuditFinding instantiates a new PolicyAuditFinding object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPolicyAuditFindingWithDefaults

`func NewPolicyAuditFindingWithDefaults() *PolicyAuditFinding`

NewPolicyAuditFindingWithDefaults instantiates a new PolicyAuditFinding object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *PolicyAuditFinding) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *PolicyAuditFinding) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *PolicyAuditFinding) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *PolicyAuditFinding) HasId() bool`

HasId returns a boolean if a field has been set.

### GetOccurredAt

`func (o *PolicyAuditFinding) GetOccurredAt() time.Time`

GetOccurredAt returns the OccurredAt field if non-nil, zero value otherwise.

### GetOccurredAtOk

`func (o *PolicyAuditFinding) GetOccurredAtOk() (*time.Time, bool)`

GetOccurredAtOk returns a tuple with the OccurredAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOccurredAt

`func (o *PolicyAuditFinding) SetOccurredAt(v time.Time)`

SetOccurredAt sets OccurredAt field to given value.

### HasOccurredAt

`func (o *PolicyAuditFinding) HasOccurredAt() bool`

HasOccurredAt returns a boolean if a field has been set.

### GetGatewayId

`func (o *PolicyAuditFinding) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *PolicyAuditFinding) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *PolicyAuditFinding) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.

### HasGatewayId

`func (o *PolicyAuditFinding) HasGatewayId() bool`

HasGatewayId returns a boolean if a field has been set.

### GetPolicyNames

`func (o *PolicyAuditFinding) GetPolicyNames() []string`

GetPolicyNames returns the PolicyNames field if non-nil, zero value otherwise.

### GetPolicyNamesOk

`func (o *PolicyAuditFinding) GetPolicyNamesOk() (*[]string, bool)`

GetPolicyNamesOk returns a tuple with the PolicyNames field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPolicyNames

`func (o *PolicyAuditFinding) SetPolicyNames(v []string)`

SetPolicyNames sets PolicyNames field to given value.

### HasPolicyNames

`func (o *PolicyAuditFinding) HasPolicyNames() bool`

HasPolicyNames returns a boolean if a field has been set.

### GetBundleRevision

`func (o *PolicyAuditFinding) GetBundleRevision() string`

GetBundleRevision returns the BundleRevision field if non-nil, zero value otherwise.

### GetBundleRevisionOk

`func (o *PolicyAuditFinding) GetBundleRevisionOk() (*string, bool)`

GetBundleRevisionOk returns a tuple with the BundleRevision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBundleRevision

`func (o *PolicyAuditFinding) SetBundleRevision(v string)`

SetBundleRevision sets BundleRevision field to given value.

### HasBundleRevision

`func (o *PolicyAuditFinding) HasBundleRevision() bool`

HasBundleRevision returns a boolean if a field has been set.

### GetPrincipalSubject

`func (o *PolicyAuditFinding) GetPrincipalSubject() string`

GetPrincipalSubject returns the PrincipalSubject field if non-nil, zero value otherwise.

### GetPrincipalSubjectOk

`func (o *PolicyAuditFinding) GetPrincipalSubjectOk() (*string, bool)`

GetPrincipalSubjectOk returns a tuple with the PrincipalSubject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPrincipalSubject

`func (o *PolicyAuditFinding) SetPrincipalSubject(v string)`

SetPrincipalSubject sets PrincipalSubject field to given value.

### HasPrincipalSubject

`func (o *PolicyAuditFinding) HasPrincipalSubject() bool`

HasPrincipalSubject returns a boolean if a field has been set.

### GetOperation

`func (o *PolicyAuditFinding) GetOperation() string`

GetOperation returns the Operation field if non-nil, zero value otherwise.

### GetOperationOk

`func (o *PolicyAuditFinding) GetOperationOk() (*string, bool)`

GetOperationOk returns a tuple with the Operation field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOperation

`func (o *PolicyAuditFinding) SetOperation(v string)`

SetOperation sets Operation field to given value.

### HasOperation

`func (o *PolicyAuditFinding) HasOperation() bool`

HasOperation returns a boolean if a field has been set.

### GetOutcome

`func (o *PolicyAuditFinding) GetOutcome() string`

GetOutcome returns the Outcome field if non-nil, zero value otherwise.

### GetOutcomeOk

`func (o *PolicyAuditFinding) GetOutcomeOk() (*string, bool)`

GetOutcomeOk returns a tuple with the Outcome field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOutcome

`func (o *PolicyAuditFinding) SetOutcome(v string)`

SetOutcome sets Outcome field to given value.

### HasOutcome

`func (o *PolicyAuditFinding) HasOutcome() bool`

HasOutcome returns a boolean if a field has been set.

### GetRequestSummary

`func (o *PolicyAuditFinding) GetRequestSummary() map[string]int64`

GetRequestSummary returns the RequestSummary field if non-nil, zero value otherwise.

### GetRequestSummaryOk

`func (o *PolicyAuditFinding) GetRequestSummaryOk() (*map[string]int64, bool)`

GetRequestSummaryOk returns a tuple with the RequestSummary field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRequestSummary

`func (o *PolicyAuditFinding) SetRequestSummary(v map[string]int64)`

SetRequestSummary sets RequestSummary field to given value.

### HasRequestSummary

`func (o *PolicyAuditFinding) HasRequestSummary() bool`

HasRequestSummary returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
