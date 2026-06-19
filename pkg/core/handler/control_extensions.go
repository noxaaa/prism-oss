package handler

import (
	"net/http"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

type InternalRouteHandler func(http.ResponseWriter, *http.Request, service.InternalIdentity)

type ControlRouteExtension interface {
	RegisterControlRoutes(registry ControlRouteRegistry)
}

type ControlRouteRegistry interface {
	HandleInternal(pattern string, handler InternalRouteHandler)
	ControlService() *service.ControlService
	Edition() edition.Provider
}

type controlRouteRegistry struct {
	server *ControlServer
}

func (registry controlRouteRegistry) HandleInternal(pattern string, handler InternalRouteHandler) {
	registry.server.mux.HandleFunc(pattern, registry.server.withInternalIdentity(func(response http.ResponseWriter, request *http.Request, claims auth.InternalClaims) {
		handler(response, request, internalIdentityFromClaims(claims, request))
	}))
}

func (registry controlRouteRegistry) ControlService() *service.ControlService {
	return registry.server.controlService
}

func (registry controlRouteRegistry) Edition() edition.Provider {
	return registry.server.edition
}

func WriteServiceResponse(response http.ResponseWriter, status int, value any, err error) {
	writeServiceResponse(response, status, value, err)
}
