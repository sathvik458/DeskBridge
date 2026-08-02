// Command studyhub-server is the StudyHub backend.
//
// Right now it does one thing: answer /health so I can tell from my Mac
// whether the Bahrain machine is actually up. Everything else gets added
// phase by phase.
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// listenAddr is hardcoded for now. It moves into config in the next step,
// because the Bahrain PC will not be listening on localhost forever.
const listenAddr = "localhost:8080"

// healthResponse is the body of GET /health.
//
// Using a named struct instead of a map so the shape of the response is
// obvious from the code and a typo in a key becomes a compile error.
type healthResponse struct {
	Status string `json:"status"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok"}); err != nil {
		// The status line is already sent by now, so there is nothing useful
		// to tell the client. Log it and move on.
		log.Printf("health: could not write response: %v", err)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	log.Printf("studyhub server listening on http://%s", listenAddr)

	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
