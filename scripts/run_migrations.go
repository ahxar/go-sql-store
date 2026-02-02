package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
	"github.com/safar/go-sql-store/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run scripts/run_migrations.go [up|down]")
	}

	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		log.Fatal("Direction must be 'up' or 'down'")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}

	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Fatalf("Connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Ping database: %v", err)
	}

	migrationDir := "migrations"
	files, err := os.ReadDir(migrationDir)
	if err != nil {
		log.Fatalf("Read migration directory: %v", err)
	}

	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), fmt.Sprintf(".%s.sql", direction)) {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}

	sort.Strings(migrationFiles)
	if direction == "down" {
		for i, j := 0, len(migrationFiles)-1; i < j; i, j = i+1, j-1 {
			migrationFiles[i], migrationFiles[j] = migrationFiles[j], migrationFiles[i]
		}
	}

	for _, filename := range migrationFiles {
		filePath := filepath.Join(migrationDir, filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Read migration file %s: %v", filename, err)
		}

		log.Printf("Running migration: %s", filename)
		if _, err := db.Exec(string(content)); err != nil {
			log.Fatalf("Execute migration %s: %v", filename, err)
		}
	}

	log.Printf("Successfully ran %d migration(s) %s", len(migrationFiles), direction)
}
