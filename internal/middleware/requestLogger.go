package middleware

import (
	"net/http"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Log interface {
	Info(string, ...zapcore.Field)
}

type ReqLog struct {
	log Log
}

func NewReqLog(log Log) *ReqLog {
	return &ReqLog{
		log: log,
	}
}

// RequestLogger — middleware logger for incoming HTTP requests.
func (rl *ReqLog) RequestLogger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl.log.Info("got incoming HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)
		h.ServeHTTP(w, r)
	})
}
