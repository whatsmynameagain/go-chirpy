package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/whatsmynameagain/go-chirpy/internal/database"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("failed to open database")
	}

	const rootDir = "./"
	const port = "8080"

	apiCfg := &apiConfig{
		maxChirpLength: 140,
		dbQueries:      database.New(db),
	} // fileserverHits default is 0, no need to initialize

	newMux := http.NewServeMux()
	serverStruct := &http.Server{
		Handler: newMux,
		Addr:    ":" + port,
	}

	fileServer := http.FileServer(http.Dir(rootDir))
	newMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

	newMux.HandleFunc("GET /admin/metrics", apiCfg.requestCountHandler)

	// newMux.HandleFunc("POST /admin/reset", apiCfg.resetCountHandler)
	newMux.HandleFunc("POST /admin/reset", apiCfg.resetUsers)

	newMux.HandleFunc("GET /api/healthz", readinessHandler)

	//newMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateChirpHandler)

	newMux.HandleFunc("POST /api/users", apiCfg.createUser)

	newMux.HandleFunc("POST /api/chirps", apiCfg.createChirp)
	newMux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)

	log.Printf("Serving %s on :%s\n", rootDir, port)
	err = serverStruct.ListenAndServe()
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
	dbQueries      *database.Queries
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

/*
func (cfg *apiConfig) resetCountHandler(w http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset"))
}
*/

// gonna keep adding the handler functions here for now

func checkProfanity(txt string, censor string, profanityList []string) []string {

	words := strings.Split(txt, " ")
	for i, n := range words {
		for _, p := range profanityList {
			if strings.ToLower(n) == strings.ToLower(p) {
				words[i] = censor
			}
		}
	}
	return words
}

type User struct {
	Id        uuid.UUID `json:"id"`
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

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	type usrReq struct {
		Email string `json:"email"`
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "could not read request")
		return
	}

	usrData := usrReq{}
	err = json.Unmarshal(data, &usrData)
	if err != nil {
		respondWithError(w, 400, "could not unmarshal data")
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), usrData.Email)
	if err != nil {
		respondWithError(w, 500, "could not create user")
		return
	}

	resp := User{
		Id:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	err = respondWithJSON(w, 201, resp)
}

func (cfg *apiConfig) resetUsers(w http.ResponseWriter, r *http.Request) {
	platform := os.Getenv("PLATFORM")
	if platform != "dev" {
		respondWithError(w, 403, "forbidden")
		return
	}
	cfg.dbQueries.ResetUsers(r.Context())
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Users reset"))
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	type chirpReq struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}
	type cleanedChirp struct {
		CleanedBody string `json:"cleaned_body"`
		UserId      string `json:"user_id"`
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "could not read request")
		return
	}
	chirpData := chirpReq{}
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

	// check profanity
	profList := []string{"kerfuffle", "sharbert", "fornax"}
	censor := "****"
	words := checkProfanity(chirpData.Body, censor, profList)

	cleanedBody := strings.Join(words, " ")
	newChirp := database.CreateChirpParams{
		Body:   cleanedBody,
		UserID: chirpData.UserID,
	}

	newChirpDB, err := cfg.dbQueries.CreateChirp(r.Context(), newChirp)
	if err != nil {
		fmt.Println("error creating chirp: ", err)
		respondWithError(w, 500, "failed to create chirp")
		return
	}

	responseChirp := Chirp{
		ID:        newChirpDB.UserID,
		CreatedAt: newChirpDB.CreatedAt,
		UpdatedAt: newChirpDB.UpdatedAt,
		Body:      newChirpDB.Body,
		UserID:    newChirpDB.UserID,
	}
	err = respondWithJSON(w, 201, responseChirp)
	if err != nil {
		fmt.Println("error sending response: ", err)
	}
}

func (cfg *apiConfig) getAllChirps(w http.ResponseWriter, r *http.Request) {

	dbChirps, err := cfg.dbQueries.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, "failed to fetch chirps")
	}
	// if empty
	if len(dbChirps) == 0 {
		respondWithJSON(w, 200, []Chirp{})
		return
	}

	// gotta do this because the database.Chirps struct doesn't have the JSON tags,
	// making the JSON keys be capitalized by the marshalling
	jsonChirps := []Chirp{}
	for _, dbChirp := range dbChirps {
		chirp := Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}
		jsonChirps = append(jsonChirps, chirp)
	}

	respondWithJSON(w, 200, jsonChirps)

}
