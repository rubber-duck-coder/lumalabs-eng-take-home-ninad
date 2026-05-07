package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/gateway"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: gateway.NewRouter(),
	}

	log.Printf("control plane listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}
