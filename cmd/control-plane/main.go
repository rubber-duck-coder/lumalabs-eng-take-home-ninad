package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/gateway"
	"github.com/ninadsindu/luma-gpu-control-plane/internal/reconciler"
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

	startReconciler(appStore)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: gateway.NewRouterWithStore(appStore),
	}

	log.Printf("control plane listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func startReconciler(appStore store.Store) {
	interval := 15 * time.Second
	if raw := os.Getenv("RECONCILE_INTERVAL_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			interval = time.Duration(seconds) * time.Second
		}
	}

	manager := reconciler.New(appStore, nil)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			changed, err := manager.RunOnce()
			if err != nil {
				log.Printf("reconciler tick failed: %v", err)
				continue
			}
			if changed > 0 {
				log.Printf("reconciler healed %d node(s)", changed)
			}
		}
	}()
}
