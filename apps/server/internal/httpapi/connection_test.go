package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestConnectionControllerSurvivesMiddleware(t *testing.T) {
	s := boundaryServer(t, io.Discard)
	result := make(chan error, 1)
	s.router.GET("/controller-test", func(c *gin.Context) {
		controller := connectionController(c.Request, c.Writer)
		err := controller.SetWriteDeadline(time.Now().Add(time.Second))
		if err == nil {
			err = controller.SetWriteDeadline(time.Time{})
		}
		result <- err
		c.Status(204)
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/controller-test")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if err := <-result; err != nil {
		t.Fatalf("middleware hid connection deadlines: %v", err)
	}
}
