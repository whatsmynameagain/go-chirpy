package main

import (
	"log"
	"net/http"
)

func main() {

	const rootDir = "/app/" // ?
	const port = "8080"

	newMux := http.NewServeMux()
	serverStruct := &http.Server{
		Handler: newMux,
		Addr:    ":" + port,
	}

	newMux.Handle("/", http.FileServer(http.Dir(rootDir)))

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
