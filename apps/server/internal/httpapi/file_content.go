package httpapi

import (
	"context"
	"io"
	"net/http"
	"time"
)

type fileResponseWriter struct {
	http.ResponseWriter
	ctx        context.Context
	controller *http.ResponseController
}

func (w *fileResponseWriter) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		if err := w.ctx.Err(); err != nil {
			return written, err
		}
		if err := w.controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return written, err
		}
		chunk := data
		if len(chunk) > 32<<10 {
			chunk = chunk[:32<<10]
		}
		n, err := w.ResponseWriter.Write(chunk)
		written += n
		data = data[n:]
		if err != nil {
			return written, err
		}
		if n != len(chunk) {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

type cancelableFile struct {
	io.ReadSeeker
	ctx context.Context
}

func (f cancelableFile) Read(p []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	return f.ReadSeeker.Read(p)
}
func (f cancelableFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	return f.ReadSeeker.Seek(offset, whence)
}

func serveFileContent(w http.ResponseWriter, r *http.Request, file io.ReadSeeker) {
	controller := connectionController(r, w)
	defer controller.SetWriteDeadline(time.Time{})
	writer := &fileResponseWriter{ResponseWriter: w, ctx: r.Context(), controller: controller}
	http.ServeContent(writer, r, "", time.Time{}, cancelableFile{ReadSeeker: file, ctx: r.Context()})
}
