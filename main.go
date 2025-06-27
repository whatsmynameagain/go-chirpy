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

	"github.com/whatsmynameagain/go-chirpy/internal/auth"
	"github.com/whatsmynameagain/go-chirpy/internal/database"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	env_secret := os.Getenv("SECRET")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("failed to open database")
	}

	const rootDir = "./"
	const port = "8080"

	apiCfg := &apiConfig{
		maxChirpLength: 140,
		dbQueries:      database.New(db),
		secret:         env_secret,
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
	newMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)

	newMux.HandleFunc("POST /api/login", apiCfg.loginHandler)

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
	secret         string
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
			if strings.EqualFold(n, p) {
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
	Password  string    `json:"password,omitempty"`
	Token     string    `json:"token"`
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
		Email    string `json:"email"`
		Password string `json:"password"`
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

	hashed_pw, err := auth.HashPassword(usrData.Password)
	if err != nil {
		fmt.Println("Error hashing password: ", err)
	}

	usrParam := database.CreateUserParams{
		Email:          usrData.Email,
		HashedPassword: hashed_pw,
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), usrParam)
	if err != nil {
		fmt.Println("error: ", err)
		respondWithError(w, 500, "could not create user")
		return
	}

	resp := User{
		Id:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
		// do not include pw in the response
		// Password: 	user.HashedPassword,
	}

	err = respondWithJSON(w, 201, resp)
	if err != nil {
		fmt.Println("error responding: ", err)
	}
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

	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "could not read request")
		return
	}

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "could not read JWT")
		return
	}

	userUUID, err := auth.ValidateJWT(tokenString, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "could not validate JWT")
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
		if err != nil {
			fmt.Println("error responding: ", err)
		}
		return
	}

	// check profanity
	profList := []string{"kerfuffle", "sharbert", "fornax"}
	censor := "****"
	words := checkProfanity(chirpData.Body, censor, profList)

	cleanedBody := strings.Join(words, " ")
	newChirp := database.CreateChirpParams{
		Body:   cleanedBody,
		UserID: userUUID,
	}

	newChirpDB, err := cfg.dbQueries.CreateChirp(r.Context(), newChirp)
	if err != nil {
		fmt.Println("error creating chirp: ", err)
		respondWithError(w, 500, "failed to create chirp")
		return
	}

	responseChirp := Chirp{
		ID:        newChirpDB.ID,
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

	jsonChirps := []Chirp{}
	for _, dbChirp := range dbChirps {
		chirp := dbChirpToJSONChirp(&dbChirp)
		jsonChirps = append(jsonChirps, chirp)
	}

	respondWithJSON(w, 200, jsonChirps)

}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		// debug for now, replace with http response
		fmt.Println("error converting endpoint uuid string into uuid: ", err)
		return
	}

	fetchedChirp, err := cfg.dbQueries.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		// debug for now, replace with http response
		fmt.Println("error fetching chirp from database: ", err)
		return
	}

	// if empty
	if fetchedChirp == (database.Chirp{}) {
		respondWithError(w, 404, "no chirp found with the requested ID")
		return
	}

	returnChirp := dbChirpToJSONChirp(&fetchedChirp)
	err = respondWithJSON(w, 200, returnChirp)
	if err != nil {
		fmt.Println("error responding: ", err)
	}

}

func dbChirpToJSONChirp(chrp *database.Chirp) Chirp {
	// gotta do this because the database.Chirps struct doesn't have the JSON tags,
	// making the JSON keys be capitalized by the marshalling
	return Chirp{
		ID:        chrp.ID,
		CreatedAt: chrp.CreatedAt,
		UpdatedAt: chrp.UpdatedAt,
		Body:      chrp.Body,
		UserID:    chrp.UserID,
	}

}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type loginReq struct {
		Password         string `json:"password"`
		Email            string `json:"email"`
		ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 400, "could not read request")
		return
	}
	userLogin := loginReq{}
	err = json.Unmarshal(data, &userLogin)
	if err != nil {
		respondWithError(w, 400, "could not unmarshal data")
		return
	}
	// request email and password validation goes here
	// (e.g. valid email, pw length)
	expirationTime := 0
	if userLogin.ExpiresInSeconds == nil {
		expirationTime = 3600
	} else {
		if *userLogin.ExpiresInSeconds > 3600 {
			expirationTime = 3600
		} else {
			expirationTime = *userLogin.ExpiresInSeconds
		}
	}

	userInfo, err := cfg.dbQueries.GetUserByEmail(r.Context(), userLogin.Email)
	if err != nil {
		respondWithError(w, 401, "incorrect user or password")
		return
	}

	if auth.CheckPasswordHash(userInfo.HashedPassword, userLogin.Password) != nil {
		respondWithError(w, 401, "incorrect user or password")
		return
	}

	new_token, err := auth.MakeJWT(userInfo.ID, cfg.secret, time.Duration(expirationTime)*time.Second)
	if err != nil {
		fmt.Println("error creating jwt: ", err)
	}

	respondWithJSON(w, 200, User{
		Id:        userInfo.ID,
		CreatedAt: userInfo.CreatedAt,
		UpdatedAt: userInfo.UpdatedAt,
		Email:     userInfo.Email,
		Token:     new_token,
	})
}
