package users

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type UserService interface {
	Get(ctx context.Context, id string) (*User, *errors.ServiceError)
	GetByUsername(ctx context.Context, username string) (*User, *errors.ServiceError)
	Create(ctx context.Context, user *User) (*User, *errors.ServiceError)
	Upsert(ctx context.Context, user *User) (*User, *errors.ServiceError)
	UpsertByUsername(ctx context.Context, username string, email *string, name *string) (string, error)
	All(ctx context.Context) (UserList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (UserList, *errors.ServiceError)
}

func NewUserService(userDao UserDao) UserService {
	return &sqlUserService{
		userDao: userDao,
	}
}

var _ UserService = &sqlUserService{}

type sqlUserService struct {
	userDao UserDao
}

func (s *sqlUserService) Get(ctx context.Context, id string) (*User, *errors.ServiceError) {
	user, err := s.userDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("User", "id", id, err)
	}
	return user, nil
}

func (s *sqlUserService) GetByUsername(ctx context.Context, username string) (*User, *errors.ServiceError) {
	user, err := s.userDao.GetByUsername(ctx, username)
	if err != nil {
		return nil, services.HandleGetError("User", "username", username, err)
	}
	return user, nil
}

func (s *sqlUserService) Create(ctx context.Context, user *User) (*User, *errors.ServiceError) {
	user, err := s.userDao.Create(ctx, user)
	if err != nil {
		return nil, services.HandleCreateError("User", err)
	}
	return user, nil
}

func (s *sqlUserService) Upsert(ctx context.Context, user *User) (*User, *errors.ServiceError) {
	user, err := s.userDao.Upsert(ctx, user)
	if err != nil {
		return nil, services.HandleCreateError("User", err)
	}
	return user, nil
}

func (s *sqlUserService) UpsertByUsername(ctx context.Context, username string, email *string, name *string) (string, error) {
	user := &User{
		Username: username,
		Email:    email,
		Name:     name,
	}
	user, err := s.userDao.Upsert(ctx, user)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

func (s *sqlUserService) All(ctx context.Context) (UserList, *errors.ServiceError) {
	users, err := s.userDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all users: %s", err)
	}
	return users, nil
}

func (s *sqlUserService) FindByIDs(ctx context.Context, ids []string) (UserList, *errors.ServiceError) {
	users, err := s.userDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get users: %s", err)
	}
	return users, nil
}
