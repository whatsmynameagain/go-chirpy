package main

import (
	"log"
	"net/http"
)

func main() {
	newMux := http.NewServeMux()
	serverStruct := http.Server{
		Handler: newMux,
		Addr:    ":8080",
	}

	err := serverStruct.ListenAndServe()
	// error checking because why not
	if err != nil {
		log.Fatalf("error serving")
	}
}
