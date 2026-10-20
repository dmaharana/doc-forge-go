package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docforge/internal/api"
	"docforge/internal/config"
	"docforge/internal/db"
	"docforge/internal/ingestion"
	"docforge/internal/repository"
	"docforge/internal/service"
)

func main() {
	log.Println("[DOCFORGE] Starting DocForge Backend Service...")

	cfg := config.Load()
	log.Printf("[CONFIG] Running in Port: %d | DevMode: %t | MockAtlassian: %t", cfg.Port, cfg.DevMode, cfg.MockAtlassian)

	// Database connection & migrations
	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Database connection failed: %v", err)
	}
	defer database.Close()
	log.Printf("[DB] Connected to SQLite database at: %s", cfg.DBPath)

	// Local file storage
	storage, err := ingestion.NewLocalStorage(cfg.UploadDir)
	if err != nil {
		log.Fatalf("[FATAL] File storage initialization failed: %v", err)
	}
	log.Printf("[STORAGE] Upload storage initialized at: %s", cfg.UploadDir)

	// Repositories & Services
	repo := repository.New(database)
	notifier := service.NewNotificationService(cfg)
	atlassian := service.NewAtlassianService(cfg, repo)
	genSvc := service.NewGenerationService(cfg, repo, notifier)

	// Start background generation worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	genSvc.StartWorker(ctx)

	// Handlers and Router
	authHandler := api.NewAuthHandler(cfg, repo, atlassian)
	handlers := api.NewHandlers(cfg, repo, storage, genSvc, atlassian)
	router := api.NewRouter(cfg, repo, handlers, authHandler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Server shutdown channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[DOCFORGE] Server listening on http://localhost:%d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server listen error: %v", err)
		}
	}()

	<-stop
	log.Println("[DOCFORGE] Shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	genSvc.StopWorker()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	}

	log.Println("[DOCFORGE] Server stopped successfully.")
}
