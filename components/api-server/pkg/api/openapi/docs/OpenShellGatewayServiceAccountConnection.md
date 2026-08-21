# OpenShellGatewayServiceAccountConnection

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GatewayName** | **string** |  | 
**GatewayEndpoint** | Pointer to **string** |  | [optional] 
**Issuer** | **string** |  | 
**TokenEndpoint** | **string** |  | 
**GrantType** | **string** |  | 
**ClientId** | **string** |  | 
**Audience** | **string** |  | 
**AccessTokenLifetimeSeconds** | **int32** |  | 

## Methods

### NewOpenShellGatewayServiceAccountConnection

`func NewOpenShellGatewayServiceAccountConnection(gatewayName string, issuer string, tokenEndpoint string, grantType string, clientId string, audience string, accessTokenLifetimeSeconds int32, ) *OpenShellGatewayServiceAccountConnection`

NewOpenShellGatewayServiceAccountConnection instantiates a new OpenShellGatewayServiceAccountConnection object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountConnectionWithDefaults

`func NewOpenShellGatewayServiceAccountConnectionWithDefaults() *OpenShellGatewayServiceAccountConnection`

NewOpenShellGatewayServiceAccountConnectionWithDefaults instantiates a new OpenShellGatewayServiceAccountConnection object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetGatewayName

`func (o *OpenShellGatewayServiceAccountConnection) GetGatewayName() string`

GetGatewayName returns the GatewayName field if non-nil, zero value otherwise.

### GetGatewayNameOk

`func (o *OpenShellGatewayServiceAccountConnection) GetGatewayNameOk() (*string, bool)`

GetGatewayNameOk returns a tuple with the GatewayName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayName

`func (o *OpenShellGatewayServiceAccountConnection) SetGatewayName(v string)`

SetGatewayName sets GatewayName field to given value.


### GetGatewayEndpoint

`func (o *OpenShellGatewayServiceAccountConnection) GetGatewayEndpoint() string`

GetGatewayEndpoint returns the GatewayEndpoint field if non-nil, zero value otherwise.

### GetGatewayEndpointOk

`func (o *OpenShellGatewayServiceAccountConnection) GetGatewayEndpointOk() (*string, bool)`

GetGatewayEndpointOk returns a tuple with the GatewayEndpoint field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayEndpoint

`func (o *OpenShellGatewayServiceAccountConnection) SetGatewayEndpoint(v string)`

SetGatewayEndpoint sets GatewayEndpoint field to given value.

### HasGatewayEndpoint

`func (o *OpenShellGatewayServiceAccountConnection) HasGatewayEndpoint() bool`

HasGatewayEndpoint returns a boolean if a field has been set.

### GetIssuer

`func (o *OpenShellGatewayServiceAccountConnection) GetIssuer() string`

GetIssuer returns the Issuer field if non-nil, zero value otherwise.

### GetIssuerOk

`func (o *OpenShellGatewayServiceAccountConnection) GetIssuerOk() (*string, bool)`

GetIssuerOk returns a tuple with the Issuer field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIssuer

`func (o *OpenShellGatewayServiceAccountConnection) SetIssuer(v string)`

SetIssuer sets Issuer field to given value.


### GetTokenEndpoint

`func (o *OpenShellGatewayServiceAccountConnection) GetTokenEndpoint() string`

GetTokenEndpoint returns the TokenEndpoint field if non-nil, zero value otherwise.

### GetTokenEndpointOk

`func (o *OpenShellGatewayServiceAccountConnection) GetTokenEndpointOk() (*string, bool)`

GetTokenEndpointOk returns a tuple with the TokenEndpoint field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTokenEndpoint

`func (o *OpenShellGatewayServiceAccountConnection) SetTokenEndpoint(v string)`

SetTokenEndpoint sets TokenEndpoint field to given value.


### GetGrantType

`func (o *OpenShellGatewayServiceAccountConnection) GetGrantType() string`

GetGrantType returns the GrantType field if non-nil, zero value otherwise.

### GetGrantTypeOk

`func (o *OpenShellGatewayServiceAccountConnection) GetGrantTypeOk() (*string, bool)`

GetGrantTypeOk returns a tuple with the GrantType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGrantType

`func (o *OpenShellGatewayServiceAccountConnection) SetGrantType(v string)`

SetGrantType sets GrantType field to given value.


### GetClientId

`func (o *OpenShellGatewayServiceAccountConnection) GetClientId() string`

GetClientId returns the ClientId field if non-nil, zero value otherwise.

### GetClientIdOk

`func (o *OpenShellGatewayServiceAccountConnection) GetClientIdOk() (*string, bool)`

GetClientIdOk returns a tuple with the ClientId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClientId

`func (o *OpenShellGatewayServiceAccountConnection) SetClientId(v string)`

SetClientId sets ClientId field to given value.


### GetAudience

`func (o *OpenShellGatewayServiceAccountConnection) GetAudience() string`

GetAudience returns the Audience field if non-nil, zero value otherwise.

### GetAudienceOk

`func (o *OpenShellGatewayServiceAccountConnection) GetAudienceOk() (*string, bool)`

GetAudienceOk returns a tuple with the Audience field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAudience

`func (o *OpenShellGatewayServiceAccountConnection) SetAudience(v string)`

SetAudience sets Audience field to given value.


### GetAccessTokenLifetimeSeconds

`func (o *OpenShellGatewayServiceAccountConnection) GetAccessTokenLifetimeSeconds() int32`

GetAccessTokenLifetimeSeconds returns the AccessTokenLifetimeSeconds field if non-nil, zero value otherwise.

### GetAccessTokenLifetimeSecondsOk

`func (o *OpenShellGatewayServiceAccountConnection) GetAccessTokenLifetimeSecondsOk() (*int32, bool)`

GetAccessTokenLifetimeSecondsOk returns a tuple with the AccessTokenLifetimeSeconds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAccessTokenLifetimeSeconds

`func (o *OpenShellGatewayServiceAccountConnection) SetAccessTokenLifetimeSeconds(v int32)`

SetAccessTokenLifetimeSeconds sets AccessTokenLifetimeSeconds field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


