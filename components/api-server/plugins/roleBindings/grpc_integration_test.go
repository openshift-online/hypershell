package roleBindings_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

type roleBindingBearerToken struct {
	token string
}

func (b *roleBindingBearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b *roleBindingBearerToken) RequireTransportSecurity() bool {
	return false
}

func TestGRPCWatchRoleBindingsReplaysExistingBindingsAfterEachConnect(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	h.StartControllersServer()

	account := h.NewRandAccount()
	userService := users.Service(&environments.Environment().Services)
	userID, err := userService.UpsertByUsername(context.Background(), account.Username, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	gatewayID := "gateway-before-role-binding-watch"
	rbService := roleBindings.Service(&environments.Environment().Services)
	Expect(rbService.CreateGatewayOwnerBinding(context.Background(), userID, gatewayID)).To(Succeed())

	bindings, svcErr := rbService.FindByUserID(context.Background(), userID)
	Expect(svcErr).NotTo(HaveOccurred())
	var bindingID string
	for _, binding := range bindings {
		if binding.GatewayID != nil && *binding.GatewayID == gatewayID {
			bindingID = binding.ID
			break
		}
	}
	Expect(bindingID).NotTo(BeEmpty())

	conn, err := grpc.NewClient(
		h.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&roleBindingBearerToken{token: h.CreateJWTString(account)}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		Expect(conn.Close()).To(Succeed())
	})

	for attempt := 0; attempt < 2; attempt++ {
		watchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		stream, watchErr := pb.NewRoleBindingServiceClient(conn).WatchRoleBindings(watchCtx, &pb.WatchRoleBindingsRequest{})
		Expect(watchErr).NotTo(HaveOccurred())
		_, headerErr := stream.Header()
		Expect(headerErr).NotTo(HaveOccurred())

		for {
			event, recvErr := stream.Recv()
			Expect(recvErr).NotTo(HaveOccurred())
			if event.ResourceId != bindingID {
				continue
			}

			Expect(event.Type).To(Equal(pb.EventType_EVENT_TYPE_UPDATED))
			Expect(event.RoleBinding).NotTo(BeNil())
			Expect(event.RoleBinding.RoleName).To(Equal("gateway:owner"))
			Expect(event.RoleBinding.Username).To(Equal(account.Username))
			Expect(event.RoleBinding.GetGatewayId()).To(Equal(gatewayID))
			break
		}
		cancel()
	}
}
