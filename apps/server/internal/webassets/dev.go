//go:build dev

package webassets

import "net/http"

// Next dev serves the frontend and proxies API requests to Go in development.
func Embedded() (http.Handler, error) { return nil, nil }
