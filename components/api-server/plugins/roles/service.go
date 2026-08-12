package roles

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type RoleService interface {
	Get(ctx context.Context, id string) (*Role, *errors.ServiceError)
	GetByName(ctx context.Context, name string) (*Role, *errors.ServiceError)
	All(ctx context.Context) (RoleList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (RoleList, *errors.ServiceError)
}

func NewRoleService(roleDao RoleDao) RoleService {
	return &sqlRoleService{
		roleDao: roleDao,
	}
}

var _ RoleService = &sqlRoleService{}

type sqlRoleService struct {
	roleDao RoleDao
}

func (s *sqlRoleService) Get(ctx context.Context, id string) (*Role, *errors.ServiceError) {
	role, err := s.roleDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("Role", "id", id, err)
	}
	return role, nil
}

func (s *sqlRoleService) GetByName(ctx context.Context, name string) (*Role, *errors.ServiceError) {
	role, err := s.roleDao.GetByName(ctx, name)
	if err != nil {
		return nil, services.HandleGetError("Role", "name", name, err)
	}
	return role, nil
}

func (s *sqlRoleService) All(ctx context.Context) (RoleList, *errors.ServiceError) {
	roles, err := s.roleDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all roles: %s", err)
	}
	return roles, nil
}

func (s *sqlRoleService) FindByIDs(ctx context.Context, ids []string) (RoleList, *errors.ServiceError) {
	roles, err := s.roleDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get roles: %s", err)
	}
	return roles, nil
}
