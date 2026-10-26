package main

import (
	"database/sql"
	"log"

	"jira-u-bot/config"
	"jira-u-bot/migrations"

	_ "modernc.org/sqlite"
)

func main() {
	var cfg config.AppConfig
	if err := config.LoadConfig(&cfg); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := sql.Open("sqlite", cfg.SQLitePath)
	if err != nil {
		log.Fatalf("failed to open sqlite database at %s: %v", cfg.SQLitePath, err)
	}
	defer db.Close()

	if err := migrations.Apply(db); err != nil {
		log.Fatalf("failed to apply migrations: %v", err)
	}

	router := SetupRouter(db)

	// Background poller goroutine is wired in here starting Chapter 1d.

	log.Printf("jira-u-bot listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
