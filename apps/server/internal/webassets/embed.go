//go:build !dev

package webassets

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var embedded embed.FS

func Embedded() (http.Handler, error) {
	source, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, err
	}
	return New(source)
}
