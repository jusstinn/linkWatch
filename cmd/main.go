package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "modernc.org/sqlite" // SQLite driver

	"github.com/you/linkwatch/internal/checker"
	"github.com/you/linkwatch/internal/config"
	httpapi "github.com/you/linkwatch/internal/http" // renamed for clarity
	"github.com/you/linkwatch/internal/store"
)

func main() {
	cfg := loadConfig()
	db := connectDatabase(cfg.DatabaseURL)
	defer db.Close()

	runMigrations(db)

	st := store.NewSQLiteStore(db)
	server := httpapi.NewServer(st)
	chk := checker.NewChecker(st, cfg.CheckInterval, cfg.HTTPTimeout, cfg.ShutdownGrace, cfg.MaxConcurrency)

	chk.Start()
	startHTTPServer(server)

	waitForShutdown(chk)
}

func loadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	log.Printf("Loaded config: %+v", cfg)
	return cfg
}

func connectDatabase(dsn string) *sql.DB {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	log.Println("Database connection established")
	return db
}

func runMigrations(db *sql.DB) {
	log.Println("Starting migrations...")

	if _, err := os.Stat("migrations"); os.IsNotExist(err) {
		log.Fatal("migrations directory does not exist")
	}

	err := store.RunMigrations(db, "migrations")
	if err != nil {
		log.Printf("Migration failed: %v", err)
		log.Fatal("Cannot continue without database schema")
	}
	log.Println("Database migrations complete")
}

func startHTTPServer(server *httpapi.Server) {
	addr := ":8080"
	log.Printf("Starting HTTP server on %s", addr)

	go func() {
		if err := http.ListenAndServe(addr, server.Router()); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server failed to start: %v", err)
			os.Exit(1)
		}
	}()
}

func waitForShutdown(chk *checker.Checker) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	<-sigChan
	log.Println("Shutdown signal received...")

	chk.Shutdown()

	log.Println("Shutdown complete")
}
