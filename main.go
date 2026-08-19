package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	myHandler := http.NewServeMux()
	readiness_endpoint := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type:", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}

	myHandler.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))
	myHandler.HandleFunc("/healthz", readiness_endpoint)

	s := &http.Server{
		Addr:           ":8080",
		Handler:        myHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())
}
