package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

func main() {

	const rootDir = "./"
	const port = "8080"

	apiCfg := &apiConfig{
		maxChirpLength: 140,
	} // fileserverHits default is 0, no need to initialize

	newMux := http.NewServeMux()
	serverStruct := &http.Server{
		Handler: newMux,
		Addr:    ":" + port,
	}

	fileServer := http.FileServer(http.Dir(rootDir))
	newMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

	newMux.HandleFunc("GET /admin/metrics", apiCfg.requestCountHandler)

	newMux.HandleFunc("POST /admin/reset", apiCfg.resetCountHandler)

	newMux.HandleFunc("GET /api/healthz", readinessHandler)

	newMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateChirpHandler)

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
	maxChirpLength uint8 //test
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) requestCountHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	respText := fmt.Sprintf(`
		<html>
		<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
		</body>
		</html>`,
		cfg.fileserverHits.Load())
	w.Write([]byte(respText))
}

func (cfg *apiConfig) resetCountHandler(w http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset"))
}

// gonna keep adding the handler functions here for now
func (cfg *apiConfig) validateChirpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	type chirp struct {
		Body string `json:"body"`
	}
	type cleanedResp struct {
		CleanedBody string `json:"cleaned_body"`
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "could not read request")
		return
	}
	chirpData := chirp{}
	err = json.Unmarshal(data, &chirpData)
	if err != nil {
		respondWithError(w, 400, "could not unmarshal data")
		return
	}

	// check length
	if len(chirpData.Body) > int(cfg.maxChirpLength) {
		msg := fmt.Sprintf("chirp is too long (max: %d)", cfg.maxChirpLength)
		err = respondWithError(w, 400, msg)
		return
	}

	words := checkProfanity(chirpData.Body)

	resp := cleanedResp{
		CleanedBody: strings.Join(words, " "),
	}

	err = respondWithJSON(w, 200, resp)

	if err != nil {
		log.Fatal("failed to respond to chirp validation request")
		return
	}
}

func checkProfanity(txt string) []string {
	profList := [3]string{"kerfuffle", "sharbert", "fornax"}
	censor := "****"
	words := strings.Split(txt, " ")

	for i, n := range words {
		for _, p := range profList {
			if strings.ToLower(n) == strings.ToLower(p) {
				words[i] = censor
			}
		}
	}
	return words
}
