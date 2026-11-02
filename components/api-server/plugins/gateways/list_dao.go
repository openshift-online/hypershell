package gateways

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	trexdao "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/dao"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

const gatewayCreatorJoinTemplate = `LEFT JOIN (
	SELECT DISTINCT ON (role_bindings.gateway_id)
		role_bindings.gateway_id,
		users.username
	FROM role_bindings
	JOIN roles ON roles.id = role_bindings.role_id AND roles.deleted_at IS NULL
	JOIN users ON users.id = role_bindings.user_id AND users.deleted_at IS NULL
	WHERE role_bindings.deleted_at IS NULL AND roles.name = '%s'
	ORDER BY role_bindings.gateway_id, role_bindings.created_at ASC, role_bindings.id ASC
) gateway_creators ON gateway_creators.gateway_id = gateways.id`

var gatewayCreatorJoin = fmt.Sprintf(gatewayCreatorJoinTemplate, roles.RoleGatewayOwner)

type gatewayListDao struct {
	trexdao.GenericDao
}

func newGatewayListService(sessionFactory *db.SessionFactory) services.GenericService {
	// Wrap with the shared search-column validator so an unknown search column is a
	// clean 400 before it reaches SQL, never a transaction-poisoning 500 /
	// SQLSTATE 25P02. Gateways use a bespoke list DAO (the creator join) rather
	// than the shared generic.Service, so the validation must be applied here too
	// (issue #871).
	return extsearch.NewValidatingService(services.NewGenericService(&gatewayListDao{
		GenericDao: trexdao.NewGenericDao(sessionFactory),
	}))
}

func (d *gatewayListDao) GetInstanceDao(ctx context.Context, model interface{}) trexdao.GenericDao {
	return &gatewayListDao{GenericDao: d.GenericDao.GetInstanceDao(ctx, model)}
}

func (d *gatewayListDao) OrderBy(orderBy string) {
	field, direction, found := strings.Cut(orderBy, " ")
	if !found || field != "created_by" || (direction != "asc" && direction != "desc") {
		d.GenericDao.OrderBy(orderBy)
		return
	}

	d.Joins(gatewayCreatorJoin)
	d.GenericDao.OrderBy("gateway_creators.username " + direction)
	d.GenericDao.OrderBy("gateways.id asc")
}
