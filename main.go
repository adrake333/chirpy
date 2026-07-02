package main




import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)




type apiConfig struct {
	fileserverHits		atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(fmt.Sprintf("Hits: %v\n", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	cfg.fileserverHits.Store(0)
}




func main() {
apiCfg := apiConfig{}

mux := http.NewServeMux()

httpServer := http.Server{
	Addr:		":8080",
	Handler:	mux,
}

mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
})

mux.HandleFunc("/metrics", apiCfg.handlerMetrics)

mux.HandleFunc("/reset", apiCfg.handlerReset)

mux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))



log.Fatal(httpServer.ListenAndServe())
}
