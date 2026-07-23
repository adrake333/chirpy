package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/adrake333/chirpy/internal/auth"
	"github.com/adrake333/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits	atomic.Int32
	dbQueries	*database.Queries
	platform	string
	jwtSecret	string
	polkaKey	string
}

type requestBody struct {
	Body string `json:"body"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	CleanedBody string `json:"cleaned_body"`
}

type User struct {
	ID        	uuid.UUID 	`json:"id"`
	CreatedAt 	time.Time 	`json:"created_at"`
	UpdatedAt 	time.Time 	`json:"updated_at"`
	Email     	string    	`json:"email"`
	IsChirpyRed	bool		`json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type userRequest struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

type loginRequest struct {
	userRequest
}

type loginResponse struct {
	User
	Token string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	Token	string	`json:"token"`
}

type chirpRequest struct {
	Body   string    `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type webhookRequest struct {
	Event	string		`json:"event"`
	Data	webhookData	`json:"data"`
}

type webhookData struct {
	UserID	string	`json:"user_id"`
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	errResp := errorResponse{Error: msg}
	dat, err := json.Marshal(errResp)
	if err != nil {
		log.Printf("Error marshaling data: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func toUser(dbU database.User) User {
	return User{
		ID:       	dbU.ID,
		CreatedAt: 	dbU.CreatedAt,
		UpdatedAt: 	dbU.UpdatedAt,
		Email:     	dbU.Email,
		IsChirpyRed:	dbU.IsChirpyRed,
	}
}

func toChirp(dbC database.Chirp) Chirp {
	return Chirp{
		ID:        dbC.ID,
		CreatedAt: dbC.CreatedAt,
		UpdatedAt: dbC.UpdatedAt,
		Body:      dbC.Body,
		UserID:    dbC.UserID,
	}
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<html>
		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %d times!</p>
		</body>
	</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error retrieving bearer token: %s", err)
		respondWithError(w, 401, "error retrieving bearer token")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		log.Printf("Error validating token: %s", err)
		respondWithError(w, 401, "unauthorized")
		return
	}
	decoder := json.NewDecoder(r.Body)
	params := chirpRequest{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, 400, "Something went wrong")
		return
	}
	cleanedBody, err := validateChirp(params.Body)
	if err != nil {
		log.Printf("Error validating chirp: %s", err)
		respondWithError(w, 400, "Chirp is too long")
		return
	}
	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleanedBody,
		UserID: userID,
	})
	if err != nil {
		log.Printf("Error storing chirp: %s", err)
		respondWithError(w, 500, "Chirp failed to store in database")
		return
	}
	newChirp := toChirp(chirp)
	dat, err := json.Marshal(newChirp)
	if err != nil {
		log.Printf("Error marshaling chirp: %s", err)
		respondWithError(w, 500, "Error marshaling chirp")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func validateChirp(body string) (string, error) {
	if len(body) > 140 {
		return "", errors.New("Chirp is too long")
	}
	cleanedBody := profaneReplace(body)
	return cleanedBody, nil
}

func profaneReplace(words string) string {
	split := strings.Split(words, " ")
	for i, word := range split {
		lowered := strings.ToLower(word)
		switch lowered {
		case "kerfuffle", "sharbert", "fornax":
			split[i] = "****"
		default:
			continue
		}
	}
	cleaned := strings.Join(split, " ")
	return cleaned
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	req := userRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		log.Printf("Error decoding user: %s", err)
		errResp := errorResponse{Error: "Something went wrong"}
		dat, err := json.Marshal(errResp)
		if err != nil {
			log.Printf("Error marshaling data: %s", err)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(dat)
		return
	}
	hashPass, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		return
	}
	user, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		HashedPassword: hashPass,
		Email:          req.Email,
	})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		w.WriteHeader(500)
		return
	}
	newUser := toUser(user)
	log.Print("Create User Success")
	dat, err := json.Marshal(newUser)
	if err != nil {
		log.Printf("Error marshaling user: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		log.Print("Forbidden")
		w.WriteHeader(403)
		return
	}
	err := cfg.dbQueries.ResetUsers(r.Context())
	if err != nil {
		log.Printf("Error resetting users: %s", err)
		w.WriteHeader(500)
		return
	}
	cfg.fileserverHits.Store(0)
	w.WriteHeader(200)
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	dbchirps, err := cfg.dbQueries.GetChirps(r.Context())
	var chirps []Chirp
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		respondWithError(w, 500, "Failed to retrieve chirps")
		return
	}
	for _, dbchirp := range dbchirps {
		chirp := toChirp(dbchirp)
		chirps = append(chirps, chirp)
	}
	dat, err := json.Marshal(chirps)
	if err != nil {
		log.Printf("Error marshaling chirps: %s", err)
		respondWithError(w, 500, "Failed to marshal chirps")
		return
	}
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerGetOneChirp(w http.ResponseWriter, r *http.Request) {
	idString := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(idString)
	if err != nil {
		log.Printf("Error parsing chirp uuid: %s", err)
		respondWithError(w, 400, "Failed to parse uuid")
		return
	}
	dbChirp, err := cfg.dbQueries.GetOneChirp(r.Context(), chirpUUID)
	if err != nil {
		log.Printf("Error finding chirp: %s", err)
		respondWithError(w, 404, "Failed to find chirp")
		return
	}
	chirp := toChirp(dbChirp)
	dat, err := json.Marshal(chirp)
	if err != nil {
		log.Printf("Error marshaling chirp: %s", err)
		respondWithError(w, 500, "Failed to marshal chirp")
		return
	}
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	req := loginRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		log.Printf("Error decoding user: %s", err)
		respondWithError(w, 500, "Something went wrong")
		return
	}
	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		log.Print("Incorrect email or password")
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	match, err := auth.CheckPasswordHash(req.Password, dbUser.HashedPassword)
	if err != nil {
		log.Printf("Incorrect email or password")
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	if match == false {
		log.Printf("Incorrect email or password")
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	jwt, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		log.Printf("error creating JWT: %s", err)
		respondWithError(w, 500, "error creating JWT")
		return
	}
	refreshToken := auth.MakeRefreshToken()
	user := toUser(dbUser)
	_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token: 		refreshToken,
		UserID:		user.ID,
		ExpiresAt:	time.Now().UTC().Add(time.Hour * 24 * 60),
	})
	if err != nil {
		respondWithError(w, 500, "Couldn't create refresh token")
		return
	}
	response := loginResponse{
		User:  		user,
		Token: 		jwt,
		RefreshToken:	refreshToken,
	}
	dat, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling user: %s", err)
		respondWithError(w, 500, "Failed to marshal user")
		return
	}
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	bearer, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 400, "Failed to retrieve bearer token")
		return
	}
	dbUser, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), bearer)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	jwt, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		log.Printf("error creating JWT: %s", err)
		respondWithError(w, 500, "error creating JWT")
		return
	}
	response := refreshResponse{
		Token: jwt,
	}
	dat, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling user: %s", err)
		respondWithError(w, 500, "Failed to marshal refresh")
		return
	}
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	bearer, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 400, "Failed to retrieve bearer token")
		return
	}
	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), bearer)
	if err != nil {
		respondWithError(w, 500, "Failed to revoke refresh token")
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerUpdateCredentials(w http.ResponseWriter, r *http.Request) {
	bearer, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Failed to retreieve bearer token")
		return
	}
	userID, err := auth.ValidateJWT(bearer, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Bad or missing token")
		return
	}
	decoder := json.NewDecoder(r.Body)
	req := userRequest{}
	err = decoder.Decode(&req)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}
	hashedPass, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	dbUser, err := cfg.dbQueries.UpdateCredentials(r.Context(), database.UpdateCredentialsParams{
		Email:		req.Email,
		HashedPassword:	hashedPass,
		ID:		userID,
	})
	if err != nil {
		respondWithError(w, 500, "Failed to update credentials")
		return
	}
	user := toUser(dbUser)
	dat, err := json.Marshal(user)
	if err != nil {
		respondWithError(w, 500, "Failed to marshal user")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	idString := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(idString)
	if err != nil {
		respondWithError(w, 400, "Failed to parse uuid")
		return
	}
	bearer, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Failed to retrieve bearer token")
		return
	}
	userID, err := auth.ValidateJWT(bearer, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	dbChirp, err := cfg.dbQueries.GetOneChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, "Failed to find chirp")
		return
	}
	if dbChirp.UserID != userID {
		respondWithError(w, 403, "Unauthorized")
		return
	}
	err = cfg.dbQueries.DeleteChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, "Failed to find chirp")
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerMakeRed(w http.ResponseWriter, r *http.Request) {
	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		w.WriteHeader(401)
		return
	}
	if apiKey != cfg.polkaKey {
		w.WriteHeader(401)
		return
	}
	decoder := json.NewDecoder(r.Body)
	params := webhookRequest{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	userID, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		respondWithError(w, 400, "Failed to parse uuid")
		return
	}
	_, err = cfg.dbQueries.MakeChirpyRed(r.Context(), userID)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}

	dbURL := os.Getenv("DB_URL")

	platform := os.Getenv("PLATFORM")

	jwtSecret := os.Getenv("JWT_SECRET")

	polkaKey := os.Getenv("POLKA_KEY")

	if jwtSecret == "" {
		log.Fatal("JWT_SECRET empty")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}

	dbQueries := database.New(db)

	apiCfg := apiConfig{
		dbQueries: dbQueries,
		platform:  platform,
		jwtSecret: jwtSecret,
		polkaKey:  polkaKey,
	}

	mux := http.NewServeMux()

	httpServer := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)

	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)

	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)

	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)

	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)

	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)

	mux.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)

	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)

	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetOneChirp)

	mux.HandleFunc("PUT /api/users", apiCfg.handlerUpdateCredentials)

	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDeleteChirp)

	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.handlerMakeRed)

	mux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))

	log.Fatal(httpServer.ListenAndServe())
}
