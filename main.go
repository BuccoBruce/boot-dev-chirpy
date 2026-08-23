package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/BuccoBruce/boot-dev-chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db             *database.Queries
	fileserverHits atomic.Int32
	PLATFORM       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
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

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}
	var requestJson = parameters{}
	err := json.NewDecoder(r.Body).Decode(&requestJson)
	if err != nil {
		log.Printf("Error decoding JSON: %s", err)
		w.WriteHeader(400)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), requestJson.Email)
	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
		return
	}

	responseUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	responseJson, err := json.Marshal(responseUser)
	if err != nil {
		log.Printf("Error encoding JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(responseJson)
}

func (cfg *apiConfig) deleteUsers(w http.ResponseWriter, r *http.Request) {
	if cfg.PLATFORM != "dev" {
		w.WriteHeader(403)
		return
	}
	_, err := cfg.db.DeleteUsers(r.Context())
	if err != nil {
		w.WriteHeader(500)
		log.Printf("Error processing deletion: %s", err)
		return
	}
	w.WriteHeader(200)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		w.WriteHeader(500)
		return
	}
	responseChirps := []Chirp{}

	for _, v := range chirps {
		responseChirps = append(responseChirps, Chirp{
			ID:        v.ID,
			CreatedAt: v.CreatedAt,
			UpdatedAt: v.UpdatedAt,
			Body:      v.Body,
			UserID:    v.UserID,
		})
	}
	respondWithJson(w, 201, responseChirps)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	id, err := uuid.Parse(chirpID)
	if err != nil {
		log.Printf("Invalid UUID: %s", err)
		w.WriteHeader(400)
		return
	}

	responseChirp, err := cfg.db.GetChirp(r.Context(), id)
	if err != nil {
		log.Printf("Error returning chirp: %s", err)
		w.WriteHeader(404)
		return
	}

	respondWithJson(w, 200, responseChirp)
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
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	}
	dbQueries := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.db = dbQueries
	apiCfg.PLATFORM = os.Getenv("PLATFORM")

	myHandler := http.NewServeMux()
	myHandler.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir("./app")))))
	rep := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}

	chirp := func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
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

		c, err := apiCfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   cleanedParams.Body,
			UserID: params.UserID,
		})
		if err != nil {
			log.Printf("Error creating chirp: %s", err)
			w.WriteHeader(500)
			return
		}
		respondWithJson(w, 201, c)
	}

	myHandler.Handle("GET /admin/metrics", apiCfg.getHits())
	myHandler.HandleFunc("GET /api/healthz", rep)
	myHandler.Handle("GET /admin/reset", apiCfg.resetHits())
	myHandler.HandleFunc("POST /admin/reset", apiCfg.deleteUsers)
	// myHandler.HandleFunc("POST /api/validate_chirp", chirp)
	myHandler.HandleFunc("POST /api/users", apiCfg.createUser)
	myHandler.HandleFunc("POST /api/chirps", chirp)
	myHandler.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	myHandler.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)
	s := http.Server{
		Addr:    ":8080",
		Handler: myHandler,
	}
	log.Fatal(s.ListenAndServe())
}
