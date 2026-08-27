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

	"github.com/BuccoBruce/boot-dev-chirpy/internal/auth"
	"github.com/BuccoBruce/boot-dev-chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db             *database.Queries
	fileserverHits atomic.Int32
	PLATFORM       string
	SECRET         string
	POLKA_KEY      string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
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
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	var requestJson = parameters{}
	err := json.NewDecoder(r.Body).Decode(&requestJson)
	if err != nil {
		log.Printf("Error decoding JSON: %s", err)
		w.WriteHeader(400)
		return
	}

	hashedPassword, err := auth.HashPassword(requestJson.Password)
	if err != nil {
		log.Printf("Could not hash password")
		w.WriteHeader(500)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          requestJson.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
		return
	}

	responseUser := User{
		ID:          user.ID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
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

func (cfg *apiConfig) updateUser(w http.ResponseWriter, r *http.Request) {
	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(tokenString, cfg.SECRET)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
	}

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	var params parameters

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Invalid request body")
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, "Could not hash password")
		return
	}

	user, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		respondWithError(w, 500, "Could not update user")
		return
	}

	responseUser := User{
		ID:          user.ID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	}
	respondWithJson(w, 200, responseUser)
}

func (cfg *apiConfig) polkaWebhook(w http.ResponseWriter, r *http.Request) {
	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, 401, "Could not get API key")
		return
	}

	if apiKey != cfg.POLKA_KEY {
		respondWithError(w, 401, "provided API key does not match")
		return
	}

	type webhookData struct {
		UserID uuid.UUID `json:"user_id"`
	}

	type webhookRequest struct {
		Event string      `json:"event"`
		Data  webhookData `json:"data"`
	}

	var params webhookRequest

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Invalid request body")
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}

	_, err = cfg.db.UpgradeUser(r.Context(), params.Data.UserID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, 404, "User not found")
			return
		}

		respondWithError(w, 500, "Could not upgrade user")
		return
	}

	w.WriteHeader(204)
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
	authorId := r.URL.Query().Get("author_id")
	sortOrder := r.URL.Query().Get("sort")

	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		respondWithError(w, 400, "Invalid sort parameter")
		return
	}

	var chirps []database.Chirp

	var err error

	if authorId != "" {
		id, parseErr := uuid.Parse(authorId)
		if parseErr != nil {
			respondWithError(w, 400, "Invalid author_id")
			return
		}

		chirps, err = cfg.db.GetChirpsByAuthor(r.Context(), id)
	} else {
		chirps, err = cfg.db.GetChirps(r.Context())
	}

	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		w.WriteHeader(500)
		return
	}

	if sortOrder == "desc" {
		for i, j := 0, len(chirps)-1; i < j; i, j = i+1, j-1 {
			chirps[i], chirps[j] = chirps[j], chirps[i]
		}
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

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, r *http.Request) {
	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(tokenString, cfg.SECRET)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	chirpIDString := r.PathValue("chirpID")

	chirpID, err := uuid.Parse(chirpIDString)
	if err != nil {
		respondWithError(w, 404, "Chirp not found")
		return
	}

	chirp, err := cfg.db.GetChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, "Chirp not found")
		return
	}

	if chirp.UserID != userID {
		respondWithError(w, 403, "Forbidden")
		return
	}

	err = cfg.db.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 500, "Could not delete chirp")
		return
	}

	w.WriteHeader(204)
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

func (cfg *apiConfig) login(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	var responseUser = User{}
	var requestJson = parameters{}

	err := json.NewDecoder(r.Body).Decode(&requestJson)
	if err != nil {
		log.Printf("Error decoding JSON: %s", err)
		w.WriteHeader(400)
		return
	}

	// expiresIn := time.Hour

	// if requestJson.ExpiresInSeconds > 0 {
	// 	expiresIn = time.Duration(requestJson.ExpiresInSeconds) * time.Second

	// 	if expiresIn > time.Hour {
	// 		expiresIn = time.Hour
	// 	}
	// }
	dbUser, err := cfg.db.GetUser(r.Context(), requestJson.Email)
	if err != nil {
		log.Printf("Error getting user by email %s", err)
		w.WriteHeader(500)
		return
	}

	checkBool, err := auth.CheckPasswordHash(
		requestJson.Password,
		dbUser.HashedPassword,
	)
	if err != nil {
		log.Printf("Password validation failed")
		w.WriteHeader(401)
		return
	}
	if !checkBool {
		log.Printf("Incorrect email or password")
		w.WriteHeader(401)
		return
	}

	token, err := auth.MakeJWT(
		dbUser.ID,
		cfg.SECRET,
		time.Hour,
	)
	if err != nil {
		log.Printf("Error creating JWT %v", err)
		w.WriteHeader(500)
		return
	}

	refreshToken := auth.MakeRefreshToken()

	_, err = cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    dbUser.ID,
		ExpiresAt: time.Now().UTC().Add(60 * 24 * time.Hour),
	})
	if err != nil {
		log.Printf("Error creating refresh token %v", err)
		w.WriteHeader(500)
		return
	}

	responseUser.ID = dbUser.ID
	responseUser.CreatedAt = dbUser.CreatedAt
	responseUser.UpdatedAt = dbUser.UpdatedAt
	responseUser.Email = dbUser.Email
	responseUser.Token = token
	responseUser.RefreshToken = refreshToken
	responseUser.IsChirpyRed = dbUser.IsChirpyRed

	respondWithJson(w, 200, responseUser)
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

func (cfg *apiConfig) refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	dbToken, err := cfg.db.GetUserFromRefreshToken(
		r.Context(),
		refreshToken,
	)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	if dbToken.RevokedAt.Valid {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	if time.Now().UTC().After(dbToken.ExpiresAt) {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	accessToken, err := auth.MakeJWT(
		dbToken.UserID,
		cfg.SECRET,
		time.Hour,
	)

	if err != nil {
		respondWithError(w, 500, "Could not create access token")
		return
	}

	respondWithJson(w, 200, map[string]string{
		"token": accessToken,
	})
}

func (cfg *apiConfig) revoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	err = cfg.db.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, 500, "Could not revoke token")
		return
	}

	w.WriteHeader(204)
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
	apiCfg.SECRET = os.Getenv("SECRET")
	apiCfg.POLKA_KEY = os.Getenv("POLKA_KEY")

	myHandler := http.NewServeMux()
	myHandler.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir("./app")))))
	rep := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}

	chirp := func(w http.ResponseWriter, r *http.Request) {

		tokenString, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, 401, "Unauthorized")
			return
		}

		userID, err := auth.ValidateJWT(tokenString, apiCfg.SECRET)
		if err != nil {
			respondWithError(w, 401, "Unauthorized")
			return
		}

		type parameters struct {
			Body string `json:"body"`
			// UserID uuid.UUID `json:"user_id"`
		}
		type cleanedParameters struct {
			Body string `json:"cleaned_body"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		cleanedParams := cleanedParameters{}
		err = decoder.Decode(&params)
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
			UserID: userID,
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
	myHandler.HandleFunc("PUT /api/users", apiCfg.updateUser)
	myHandler.HandleFunc("POST /api/chirps", chirp)
	myHandler.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	myHandler.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)
	myHandler.HandleFunc("POST /api/login", apiCfg.login)
	myHandler.HandleFunc("POST /api/refresh", apiCfg.refresh)
	myHandler.HandleFunc("POST /api/revoke", apiCfg.revoke)
	myHandler.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirp)
	myHandler.HandleFunc("POST /api/polka/webhooks", apiCfg.polkaWebhook)
	s := http.Server{
		Addr:    ":8080",
		Handler: myHandler,
	}
	log.Fatal(s.ListenAndServe())
}
