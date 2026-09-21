package httpapi

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
)

// SCS calls ErrorFunc on a failed commit before forwarding the handler's
// WriteHeader/Write. Keep that failure as the sole response, including when
// shutdown cancels the request while an idle-session refresh is pending.
type sessionFailureWriter struct {
	http.ResponseWriter
	failed bool
}

func (w *sessionFailureWriter) WriteHeader(status int) {
	if !w.failed {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *sessionFailureWriter) Write(body []byte) (int, error) {
	if w.failed {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}

func (w *sessionFailureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *sessionFailureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.failed {
		return nil, nil, errors.New("SESSION_FAILED")
	}
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func sessionResponseBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&sessionFailureWriter{ResponseWriter: w}, r)
	})
}

func writeSessionFailure(w http.ResponseWriter, err error) {
	status, code := 500, "SESSION_FAILED"
	if errors.Is(err, context.DeadlineExceeded) {
		status, code = 504, "REQUEST_TIMEOUT"
	} else if errors.Is(err, context.Canceled) {
		status, code = 400, "REQUEST_CANCELLED"
	}
	if boundary, ok := w.(*sessionFailureWriter); ok {
		if boundary.failed {
			return
		}
		boundary.failed = true
		writeError(boundary.ResponseWriter, status, code)
		return
	}
	writeError(w, status, code)
}
