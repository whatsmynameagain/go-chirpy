package main

import (
	"log"
	"net/http"
)

func main() {

	const rootDir = "."
	const port = "8080"

	newMux := http.NewServeMux()
	serverStruct := &http.Server{
		Handler: newMux,
		Addr:    ":" + port,
	}

	newMux.Handle("/", http.FileServer(http.Dir(rootDir)))

	log.Printf("Serving %s on :%s\n", rootDir, port)
	err := serverStruct.ListenAndServe()
	if err != nil {
		log.Fatal("error serving")
		log.Fatal(err)
	}
	// another option is doing:
	// log.Fatal(serverStruct.ListenAndServe())
}
