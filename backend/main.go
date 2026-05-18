// backend/main.go
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/autorestic/autorestic/internal/config"
	"github.com/autorestic/autorestic/internal/db"
	"github.com/autorestic/autorestic/internal/executor"
	"github.com/autorestic/autorestic/internal/handler"
	"github.com/autorestic/autorestic/internal/middleware"
	"github.com/autorestic/autorestic/internal/repository"
	"github.com/autorestic/autorestic/internal/scheduler"
	"github.com/autorestic/autorestic/internal/service"
	"github.com/autorestic/autorestic/internal/ws"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	// Open database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Run migrations
	migrationsDir := findMigrationsDir()
	if err := db.Migrate(database, migrationsDir); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations complete")

	// Create executor
	exec := executor.New(database, cfg.ResticBin)

	// Create stores
	repoStore := repository.NewRepoStore(database)
	logStore := repository.NewLogStore(database)
	taskStore := repository.NewTaskStore(database)

	// Create services
	wsHub := ws.NewHub()
	go wsHub.Run()

	repoSvc, err := service.NewRepoService(repoStore, exec, cfg.EncKeyPath, wsHub)
	if err != nil {
		log.Fatalf("Failed to create repo service: %v", err)
	}
	repoSvc.Cache().StartPlanner()
	defer repoSvc.Cache().Stop()
	logSvc := service.NewLogService(logStore, exec)
	taskSvc := service.NewTaskService(taskStore, repoSvc)
	snapshotSvc := service.NewSnapshotService(repoSvc)
	taskScheduler := scheduler.NewScheduler(taskSvc, repoSvc)
	taskScheduler.Start()
	defer taskScheduler.Stop()

	// Create handlers
	repoHandler := handler.NewRepoHandler(repoSvc)
	logHandler := handler.NewLogHandler(logSvc)
	settingsHandler := handler.NewSettingsHandler(cfg)
	taskHandler := handler.NewTaskHandler(taskSvc)
	snapshotHandler := handler.NewSnapshotHandler(snapshotSvc)
	wsHandler := handler.NewWsHandler(wsHub, cfg.AuthToken, cfg.CORSOrigins)

	// Setup Gin
	r := gin.Default()
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Fatalf("Failed to configure trusted proxies: %v", err)
	}
	middleware.SetupMiddleware(r, middleware.Options{
		AuthToken:   cfg.AuthToken,
		CORSOrigins: cfg.CORSOrigins,
	})

	// Register routes
	api := r.Group("/api/v1")
	repoHandler.Register(api)
	logHandler.Register(api)
	settingsHandler.Register(api)
	taskHandler.Register(api)
	snapshotHandler.Register(api)
	wsHandler.RegisterAPI(api)
	wsHandler.Register(r)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Printf("AutoRestic API starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		"backend/migrations",
		"/app/migrations",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(dir)
			return abs
		}
	}
	log.Fatal("Could not find migrations directory")
	return ""
}
