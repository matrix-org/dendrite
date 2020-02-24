package promhttp

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

func InstrumentHandlerCounter(v *prometheus.CounterVec, next http.Handler) http.HandlerFunc {
	return next.ServeHTTP
}

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	})
}
