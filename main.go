package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

func main() {

	const rootDir = "./"
	const port = "8080"

	apiCfg := &apiConfig{} // fileserverHits default is 0, no need to initialize

	newMux := http.NewServeMux()
	serverStruct := &http.Server{
		Handler: newMux,
		Addr:    ":" + port,
	}

	fileServer := http.FileServer(http.Dir(rootDir))
	newMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

	newMux.HandleFunc("/metrics", apiCfg.requestCountHandler)

	newMux.HandleFunc("/reset", apiCfg.resetCountHandler)

	newMux.HandleFunc("/healthz", readinessHandler)

	log.Printf("Serving %s on :%s\n", rootDir, port)
	err := serverStruct.ListenAndServe()
	if err != nil {
		log.Fatal("error serving")
		log.Fatal(err)
	}
	// another option is doing:
	// log.Fatal(serverStruct.ListenAndServe())
}

func readinessHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// ignore bytes, error return values
	w.Write([]byte("OK"))

}

type apiConfig struct {
	fileserverHits atomic.Int32
}

// still can't wrap my head around how this works
// gotta come back to it later
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) requestCountHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	respText := fmt.Sprintf("Hits: %d\n", cfg.fileserverHits.Load())
	w.Write([]byte(respText))
}

func (cfg *apiConfig) resetCountHandler(w http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset"))
}
