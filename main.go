package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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

func respondWithError(w http.ResponseWriter, code int, msg string) {
	payload := map[string]string{
		"error": msg,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)

	if err != nil {
		log.Printf("Error marshalling JSON %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func filterChirp(chirp string) string {
	badWords := []string{"kerfuffle", "sharbert", "fornax"}
	words := strings.Split(chirp, " ")
	for i, word := range words {
		for _, badWord := range badWords {
			if strings.ToLower(word) == strings.ToLower(badWord) {
				words[i] = "****"
			}
		}
	}
	censoredChirp := strings.Join(words, " ")
	return censoredChirp
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

	chirp := func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body string `json:"body"`
		}
		type cleanedParameters struct {
			Body string `json:"cleaned_body"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		cleanedParams := cleanedParameters{}
		err := decoder.Decode(&params)
		if err != nil {
			respondWithError(w, 400, "Error decoding parameters")
			return
		}
		if len(params.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}

		cleanedParams.Body = filterChirp(params.Body)
		respondWithJson(w, 200, cleanedParams)
	}

	myHandler.Handle("GET /admin/metrics", apiCfg.getHits())
	myHandler.HandleFunc("GET /api/healthz", rep)
	myHandler.Handle("GET 	/admin/reset", apiCfg.resetHits())
	myHandler.Handle("POST 	/admin/reset", apiCfg.resetHits())
	myHandler.HandleFunc("POST /api/validate_chirp", chirp)

	s := http.Server{
		Addr:    ":8080",
		Handler: myHandler,
	}
	log.Fatal(s.ListenAndServe())
}
