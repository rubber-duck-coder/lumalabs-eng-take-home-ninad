package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/gateway"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appStore, err := store.NewConfiguredStore(context.Background())
	if err != nil {
		log.Fatalf("store init failed: %v", err)
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: gateway.NewRouterWithStore(appStore),
	}

	log.Printf("control plane listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}
