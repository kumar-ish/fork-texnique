package main

import (
	"context"
	"log"
	"net/http"
)

func main() {
	// Initialize problems -- done at the start so there's not excessive latency on the first game
	GetProblems()
	println("Beginning server...")

	// Create a root ctx and a CancelFunc which can be used to cancel retentionMap goroutine
	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)

	defer cancel()

	setupAPI(ctx)

	// Serve on port :8080
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// setupAPI will start all Routes and their Handlers
func setupAPI(ctx context.Context) {

	// Create a Manager instance used to handle WebSocket Connections
	manager := NewManager(ctx)

	// Basic routes (frontend + creation of lobby)
	http.Handle("/", http.FileServer(http.Dir("./frontend/public")))
	http.HandleFunc("/createLobby", manager.createLobbyHandler)

	// Routes used for lobby
	http.HandleFunc("/login", manager.loginHandler)
	http.HandleFunc("/ws", manager.serveWS)

	// After owner has been selected
	http.HandleFunc("/getOwner", manager.getOwnerHandler)
	http.HandleFunc("/editLobby", manager.editLobbyHandler)
}
