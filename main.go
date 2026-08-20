package main

import (
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) getHits() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		myHits := cfg.fileserverHits.Load()
		result := "Hits: " + strconv.Itoa(int(myHits))

		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(result))
	})
}

func (cfg *apiConfig) resetHits() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Store(0)
	})
}

func main() {

	apiCfg := apiConfig{}
	myHandler := http.NewServeMux()
	myHandler.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir("./app")))))
	rep := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}
	myHandler.Handle("GET /api/metrics", apiCfg.getHits())
	myHandler.HandleFunc("GET /api/healthz", rep)
	myHandler.Handle("POST 	/api/reset", apiCfg.resetHits())

	s := http.Server{
		Addr:    ":8080",
		Handler: myHandler,
	}
	log.Fatal(s.ListenAndServe())
}
