package gateways

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/db/db_context"
	"gorm.io/gorm"
)

type atomicCreationKey struct{}

// gatewaySessionFactory binds every DAO involved in reference-based creation
// to the request transaction. The pinned framework factory creates a GORM
// session but does not use the SQL transaction stored in its context.
// Calls outside this path retain the framework's existing behavior.
type gatewaySessionFactory struct{ db.SessionFactory }

func (f *gatewaySessionFactory) New(ctx context.Context) *gorm.DB {
	session := f.SessionFactory.New(ctx)
	if enabled, _ := ctx.Value(atomicCreationKey{}).(bool); !enabled {
		return session
	}
	if tx, ok := db_context.Transaction(ctx); ok {
		session = session.Session(&gorm.Session{Context: ctx, NewDB: true})
		session.Statement.ConnPool = tx.Tx()
	}
	return session
}
