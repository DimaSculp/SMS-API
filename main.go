package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sms-api-service/config"
	"sms-api-service/database"
	"sms-api-service/server"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Load()

	dbConfig := database.DefaultConfig(cfg.DBPath)
	db, err := database.Init(dbConfig)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	seedData := database.DefaultSeedData()
	if err := db.Seed(ctx, seedData); err != nil {
		log.Fatal("Failed to seed data:", err)
	}

	srv := server.New(db.DB, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/GrizzlySMSbyDima.php", srv.HandleAPIRequest)

	mux.HandleFunc("/health", handleHealthCheck)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("SMS API Service starting on port %s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server error:", err)
		}
	}()

	waitForShutdown(ctx, httpServer)
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"sms-api"}`))
}

func waitForShutdown(ctx context.Context, server *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
		return
	}

	log.Println("Server stopped gracefully")
}
