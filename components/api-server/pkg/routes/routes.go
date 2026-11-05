// Package routes declares plugin REST routes on TRex's APIRouter and adds the
// HyperShell-wide route middlewares (for example OpenTelemetry route tagging).
// Plugins call routes.Resource instead of APIRouter.Resource so every declared
// route gets the same middlewares.
package routes

import (
	"github.com/gorilla/mux"

	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

var middlewares []mux.MiddlewareFunc

// RegisterMiddleware adds a middleware to every route declared through this
// package. Call it from a plugin's init(); routes are declared later, when the
// server is built. Each middleware runs after TRex's authorization.
func RegisterMiddleware(mw mux.MiddlewareFunc) {
	middlewares = append(middlewares, mw)
}

// Resource declares a subrouter for one kind: TRex authorizes every request on
// it with Resource.Kind set to kind.
func Resource(r *pkgserver.APIRouter, kind string) *mux.Router {
	sub := r.Resource(kind)
	sub.Use(middlewares...)
	return sub
}
