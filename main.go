package main

import (
	"fmt"
	"log"
	"net/http"
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
		// intHits := strconv.Itoa(int(myHits))
		result := fmt.Sprintf(`
			<html>
				<body>
    				<h1>Welcome, Chirpy Admin</h1>
        			<p>Chirpy has been visited %d times!</p>
  				</body>
			</html>
			`, int(myHits))

		w.Header().Add("Content-Type", "text/html; charset=utf-8")
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
	myHandler.Handle("GET /admin/metrics", apiCfg.getHits())
	myHandler.HandleFunc("GET /api/healthz", rep)
	myHandler.Handle("GET 	/admin/reset", apiCfg.resetHits())

	s := http.Server{
		Addr:    ":8080",
		Handler: myHandler,
	}
	log.Fatal(s.ListenAndServe())
}
