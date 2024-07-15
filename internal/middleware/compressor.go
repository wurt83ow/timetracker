package middleware

import (
	"net/http"
	"strings"

	"github.com/wurt83ow/timetracker/internal/compress"
)

func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// by default set the original http.ResponseWriter as the one
		// that will be passed to the next function
		ow := w

		// check that the client can receive compressed data in gzip format from the server
		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip {
			// wrap the original http.ResponseWriter with a new one with compression support
			cw := compress.NewCompressWriter(w)
			// change the original http.ResponseWriter to a new one
			ow = cw
			// do not forget to send all compressed data to the client after the middleware is completed
			defer cw.Close()
		}

		// check that the client sent compressed data to the server in gzip format
		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			// wrap the request body in io.Reader with decompression support
			cr, err := compress.NewCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// change the request body to a new one
			r.Body = cr
			defer cr.Close()
		}

		// transfer control to the handler
		h.ServeHTTP(ow, r)
	})
}
