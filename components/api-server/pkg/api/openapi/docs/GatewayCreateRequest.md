# GatewayCreateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** |  |
**ClusterId** | Pointer to **string** | Pin the gateway to this managed cluster. Mutually exclusive with placement. | [optional]
**DedicatedRoute** | Pointer to **bool** | Create-only. Requires the gateway:dedicated-route-creator role and opts the gateway in to a dedicated route and per-gateway wildcard certificate. The platform manages how the dedicated route and certificate are provided. | [optional]
**WorkspaceMode** | Pointer to **string** | Create-only per-gateway operator-managed workspace opt-in. The only accepted value is \&quot;operator\&quot;, which requires the gateway:operator-workspace-creator role (platform:admin also qualifies) and provisions the gateway with operator-owned workspace namespaces (OSG-OPWS-01). Omit for the default managed mode; shared and managed are platform-managed and not caller-settable. Immutable after creation. | [optional]
**Placement** | Pointer to [**GatewayPlacementIntent**](GatewayPlacementIntent.md) |  | [optional]

## Methods

### NewGatewayCreateRequest

`func NewGatewayCreateRequest(name string, ) *GatewayCreateRequest`

NewGatewayCreateRequest instantiates a new GatewayCreateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayCreateRequestWithDefaults

`func NewGatewayCreateRequestWithDefaults() *GatewayCreateRequest`

NewGatewayCreateRequestWithDefaults instantiates a new GatewayCreateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayCreateRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayCreateRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayCreateRequest) SetName(v string)`

SetName sets Name field to given value.


### GetClusterId

`func (o *GatewayCreateRequest) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *GatewayCreateRequest) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *GatewayCreateRequest) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.

### HasClusterId

`func (o *GatewayCreateRequest) HasClusterId() bool`

HasClusterId returns a boolean if a field has been set.

### GetDedicatedRoute

`func (o *GatewayCreateRequest) GetDedicatedRoute() bool`

GetDedicatedRoute returns the DedicatedRoute field if non-nil, zero value otherwise.

### GetDedicatedRouteOk

`func (o *GatewayCreateRequest) GetDedicatedRouteOk() (*bool, bool)`

GetDedicatedRouteOk returns a tuple with the DedicatedRoute field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDedicatedRoute

`func (o *GatewayCreateRequest) SetDedicatedRoute(v bool)`

SetDedicatedRoute sets DedicatedRoute field to given value.

### HasDedicatedRoute

`func (o *GatewayCreateRequest) HasDedicatedRoute() bool`

HasDedicatedRoute returns a boolean if a field has been set.

### GetWorkspaceMode

`func (o *GatewayCreateRequest) GetWorkspaceMode() string`

GetWorkspaceMode returns the WorkspaceMode field if non-nil, zero value otherwise.

### GetWorkspaceModeOk

`func (o *GatewayCreateRequest) GetWorkspaceModeOk() (*string, bool)`

GetWorkspaceModeOk returns a tuple with the WorkspaceMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWorkspaceMode

`func (o *GatewayCreateRequest) SetWorkspaceMode(v string)`

SetWorkspaceMode sets WorkspaceMode field to given value.

### HasWorkspaceMode

`func (o *GatewayCreateRequest) HasWorkspaceMode() bool`

HasWorkspaceMode returns a boolean if a field has been set.

### GetPlacement

`func (o *GatewayCreateRequest) GetPlacement() GatewayPlacementIntent`

GetPlacement returns the Placement field if non-nil, zero value otherwise.

### GetPlacementOk

`func (o *GatewayCreateRequest) GetPlacementOk() (*GatewayPlacementIntent, bool)`

GetPlacementOk returns a tuple with the Placement field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPlacement

`func (o *GatewayCreateRequest) SetPlacement(v GatewayPlacementIntent)`

SetPlacement sets Placement field to given value.

### HasPlacement

`func (o *GatewayCreateRequest) HasPlacement() bool`

HasPlacement returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
