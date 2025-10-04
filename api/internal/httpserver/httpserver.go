package httpserver

import (
	"log"
	"net/http"
)

func StartHTTP(addr, healthzBody string) error {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(healthzBody))
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("telegram webhook bot"))
	})
	log.Printf("listening on %s", addr)
	return http.ListenAndServe(addr, nil)
}
