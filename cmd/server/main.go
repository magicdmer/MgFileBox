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

	"mgfilebox/internal/config"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/service"
	"mgfilebox/internal/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("MgFileBox %s\n", version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	repo, err := repository.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("open repository: %v", err)
	}
	defer repo.Close()

	svc := service.New(repo, cfg)

	server, err := web.NewServer(cfg, svc)
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	runCleanupWorker(svc, cfg.CleanupInterval)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Engine(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("MgFileBox %s listening on http://localhost:%s", version, cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	waitForShutdown(httpServer)
}

func runCleanupWorker(svc *service.Service, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if err := svc.CleanupExpired(context.Background()); err != nil {
				log.Printf("cleanup failed: %v", err)
			}
		}
	}()
}

func waitForShutdown(server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
