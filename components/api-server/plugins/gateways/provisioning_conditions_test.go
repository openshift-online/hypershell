package gateways_test

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/resty.v1"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestGRPCProvisioningConditionsRoundTrip(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-roundtrip",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	Expect(created.Gateway.ProvisioningConditions).To(BeEmpty(),
		"newly created gateway must have no provisioning conditions")

	conditions := []*pb.ProvisioningCondition{
		{
			Type:            "EnvironmentReady",
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE,
			Message:         "",
		},
		{
			Type:            "DatabaseReady",
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS,
			Message:         "",
		},
		{
			Type:            "GatewayDeployed",
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING,
			Message:         "",
		},
		{
			Type:            "GatewayHealthy",
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING,
			Message:         "",
		},
	}

	updated, err := grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: conditions,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(updated.Gateway.ProvisioningConditions).To(HaveLen(4))

	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.ProvisioningConditions).To(HaveLen(4))

	Expect(got.Gateway.ProvisioningConditions[0].Type).To(Equal("EnvironmentReady"))
	Expect(got.Gateway.ProvisioningConditions[0].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE))

	Expect(got.Gateway.ProvisioningConditions[1].Type).To(Equal("DatabaseReady"))
	Expect(got.Gateway.ProvisioningConditions[1].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS))

	Expect(got.Gateway.ProvisioningConditions[2].Type).To(Equal("GatewayDeployed"))
	Expect(got.Gateway.ProvisioningConditions[2].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING))

	Expect(got.Gateway.ProvisioningConditions[3].Type).To(Equal("GatewayHealthy"))
	Expect(got.Gateway.ProvisioningConditions[3].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING))
}

func TestGRPCProvisioningConditionsProgressionToComplete(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-progression",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	allPending := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: allPending,
	})
	Expect(err).NotTo(HaveOccurred())

	allComplete := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
	}
	runningPhase := "Running"
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		Phase:                  &runningPhase,
		ProvisioningConditions: allComplete,
	})
	Expect(err).NotTo(HaveOccurred())

	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetPhase()).To(Equal("Running"))
	for _, c := range got.Gateway.ProvisioningConditions {
		Expect(c.ConditionStatus).To(
			Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE),
			"condition %s should be Complete", c.Type)
	}
}

func TestGRPCProvisioningConditionsWithIdP(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-with-idp",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	withIdP := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "IdentityProviderReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: withIdP,
	})
	Expect(err).NotTo(HaveOccurred())

	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.ProvisioningConditions).To(HaveLen(5))
	Expect(got.Gateway.ProvisioningConditions[2].Type).To(Equal("IdentityProviderReady"))
	Expect(got.Gateway.ProvisioningConditions[2].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS))
}

func TestGRPCProvisioningConditionsFailedWithMessage(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-failed-msg",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	failureMessage := "Database provisioning failed - the database service is unavailable"
	failedPhase := "Failed"
	failedConditions := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{
			Type:            "DatabaseReady",
			ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED,
			Message:         failureMessage,
		},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		Phase:                  &failedPhase,
		ProvisioningConditions: failedConditions,
	})
	Expect(err).NotTo(HaveOccurred())

	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.GetPhase()).To(Equal("Failed"))
	Expect(got.Gateway.ProvisioningConditions[1].Type).To(Equal("DatabaseReady"))
	Expect(got.Gateway.ProvisioningConditions[1].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_FAILED))
	Expect(got.Gateway.ProvisioningConditions[1].Message).To(Equal(failureMessage))
}

func TestRESTGatewayReturnsProvisioningConditions(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-rest-get",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	conditions := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: conditions,
	})
	Expect(err).NotTo(HaveOccurred())

	gatewayOutput, resp, err := client.DefaultAPI.GetGateway(ctx, gatewayID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	restConditions := gatewayOutput.GetProvisioningConditions()
	Expect(restConditions).To(HaveLen(4))

	Expect(restConditions[0].GetType()).To(Equal("EnvironmentReady"))
	Expect(restConditions[0].GetConditionStatus()).To(Equal("Complete"))

	Expect(restConditions[1].GetType()).To(Equal("DatabaseReady"))
	Expect(restConditions[1].GetConditionStatus()).To(Equal("InProgress"))

	Expect(restConditions[2].GetType()).To(Equal("GatewayDeployed"))
	Expect(restConditions[2].GetConditionStatus()).To(Equal("Pending"))

	Expect(restConditions[3].GetType()).To(Equal("GatewayHealthy"))
	Expect(restConditions[3].GetConditionStatus()).To(Equal("Pending"))
}

func TestRESTGatewayListIncludesProvisioningConditions(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-rest-list",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	conditions := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: conditions,
	})
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id = '%s'", gatewayID)
	list, resp, err := client.DefaultAPI.ListGateways(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list.Items).To(HaveLen(1))

	restConditions := list.Items[0].GetProvisioningConditions()
	Expect(restConditions).To(HaveLen(4))
	for _, c := range restConditions {
		Expect(c.GetConditionStatus()).To(Equal("Complete"))
	}
}

func TestRESTProvisioningConditionsNotSettableViaPatch(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := ctx.Value(openapi.ContextAccessToken)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(map[string]interface{}{
			"provisioning_conditions": []map[string]string{
				{"type": "EnvironmentReady", "condition_status": "Complete", "message": ""},
			},
		}).
		Patch(h.RestURL(fmt.Sprintf("/gateways/%s", gatewayModel.ID)))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK),
		"PATCH with provisioning_conditions should succeed but ignore the field")
}

func TestGRPCProvisioningConditionsPreservedOnUnrelatedUpdate(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := h.CreateJWTString(account)

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerToken{token: jwtToken}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	grpcClient := pb.NewGatewayServiceClient(conn)

	phase := "Provisioning"
	created, err := grpcClient.CreateGateway(ctx, &pb.CreateGatewayRequest{
		Name:       "conditions-preserve",
		ClusterId:  "test-cluster",
		ReleaseId:  "test-release",
		DatabaseId: "test-db",
		Phase:      &phase,
	})
	Expect(err).NotTo(HaveOccurred())
	gatewayID := created.Gateway.Metadata.Id

	conditions := []*pb.ProvisioningCondition{
		{Type: "EnvironmentReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE},
		{Type: "DatabaseReady", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS},
		{Type: "GatewayDeployed", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
		{Type: "GatewayHealthy", ConditionStatus: pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_PENDING},
	}
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:                     gatewayID,
		ProvisioningConditions: conditions,
	})
	Expect(err).NotTo(HaveOccurred())

	newName := "conditions-preserve-renamed"
	_, err = grpcClient.UpdateGateway(ctx, &pb.UpdateGatewayRequest{
		Id:   gatewayID,
		Name: &newName,
	})
	Expect(err).NotTo(HaveOccurred())

	got, err := grpcClient.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	Expect(err).NotTo(HaveOccurred())
	Expect(got.Gateway.Name).To(Equal(newName))
	Expect(got.Gateway.ProvisioningConditions).To(HaveLen(4),
		"provisioning conditions must be preserved when updating unrelated fields")
	Expect(got.Gateway.ProvisioningConditions[0].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_COMPLETE))
	Expect(got.Gateway.ProvisioningConditions[1].ConditionStatus).To(
		Equal(pb.ProvisioningConditionStatus_PROVISIONING_CONDITION_STATUS_IN_PROGRESS))
}
