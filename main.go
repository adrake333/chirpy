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

	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
	"github.com/adrake333/chirpy/internal/database"
	"github.com/google/uuid"

)




type apiConfig struct {
	fileserverHits		atomic.Int32
	dbQueries		*database.Queries
	platform		string
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
	ID		uuid.UUID	`json:"id"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
	Email		string		`json:"email"`
}

type Chirp struct {
	ID		uuid.UUID	`json:"id"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
	Body		string		`json:"body"`
	UserID		uuid.UUID	`json:"user_id"`
}

type userRequest struct {
	Email	string	`json:"email"`
}

type chirpRequest struct {
	Body	string		`json:"body"`
	UserID	uuid.UUID	`json:"user_id"`
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
		ID:		dbU.ID,
		CreatedAt:	dbU.CreatedAt,
		UpdatedAt:	dbU.UpdatedAt,
		Email:		dbU.Email,
	}
}

func toChirp(dbC database.Chirp) Chirp {
	return Chirp{
		ID:		dbC.ID,
		CreatedAt:	dbC.CreatedAt,
		UpdatedAt:	dbC.UpdatedAt,
		Body:		dbC.Body,
		UserID:		dbC.UserID,
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
	decoder := json.NewDecoder(r.Body)
	params := chirpRequest{}
	err := decoder.Decode(&params)
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
		Body: cleanedBody,
		UserID: params.UserID,
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
	return
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
	user, err := cfg.dbQueries.CreateUser(r.Context(), req.Email)
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
	return
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
	return
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
	return
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
	return
}




func main() {

err := godotenv.Load()
if err != nil {
	log.Printf("Error: %s", err)
	return
}

dbURL := os.Getenv("DB_URL")

platform := os.Getenv("PLATFORM")

db, err := sql.Open("postgres", dbURL)
if err != nil {
	log.Printf("Error: %s", err)
	return
}

dbQueries := database.New(db)

apiCfg := apiConfig{
	dbQueries:	dbQueries,
	platform:	platform,
}

mux := http.NewServeMux()

httpServer := http.Server{
	Addr:		":8080",
	Handler:	mux,
}

mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
})

mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)

mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)

mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)

mux.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)

mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)

mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetOneChirp)

mux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))



log.Fatal(httpServer.ListenAndServe())
}
