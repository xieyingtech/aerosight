package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type cancelAfterWrite struct {
	*httptest.ResponseRecorder
	cancel    context.CancelFunc
	deadlines int
}

func (w *cancelAfterWrite) SetWriteDeadline(time.Time) error { w.deadlines++; return nil }
func (w *cancelAfterWrite) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	w.cancel()
	return n, err
}

func TestFileTransferStopsAtCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &cancelAfterWrite{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	w := &fileResponseWriter{ResponseWriter: out, ctx: ctx, controller: http.NewResponseController(out)}
	n, err := w.Write(bytes.Repeat([]byte("a"), 100<<10))
	if n != 32<<10 || !errors.Is(err, context.Canceled) || out.Body.Len() != 32<<10 || out.deadlines != 1 {
		t.Fatalf("continued transfer: %d %v bytes=%d deadlines=%d", n, err, out.Body.Len(), out.deadlines)
	}
	reader := cancelableFile{ReadSeeker: bytes.NewReader([]byte("private")), ctx: ctx}
	if n, err := reader.Read(make([]byte, 20)); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("read after cancellation %d %v", n, err)
	}
}
