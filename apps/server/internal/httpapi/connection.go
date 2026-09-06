package httpapi

import (
	"context"
	"net/http"
)

type connectionControllerKey struct{}

func withConnectionController(r *http.Request, w http.ResponseWriter) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), connectionControllerKey{}, http.NewResponseController(w)))
}
func connectionController(r *http.Request, w http.ResponseWriter) *http.ResponseController {
	if controller, ok := r.Context().Value(connectionControllerKey{}).(*http.ResponseController); ok {
		return controller
	}
	return http.NewResponseController(w)
}
