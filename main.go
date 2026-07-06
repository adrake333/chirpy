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
	fileserverHits		atomic.Int32
}

type requestBody struct {
	Body string `json:"body"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	Valid bool `json:"valid"`
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

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	cfg.fileserverHits.Store(0)
}

func (cfg *apiConfig) handlerValidate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := requestBody{}
	err:= decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
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
	if len(params.Body) > 140 {
		log.Print("Chirp is too long")
		errResp := errorResponse{Error: "Chirp is too long"}
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
	log.Print("Chirp Success")
	succResp := successResponse{Valid: true}
	dat, err := json.Marshal(succResp)
	if err != nil {
		log.Printf("Error marshaling data: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
	return
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




func main() {
apiCfg := apiConfig{}

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

mux.HandleFunc("POST /api/validate_chirp", apiCfg.handlerValidate)

mux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))



log.Fatal(httpServer.ListenAndServe())
}
