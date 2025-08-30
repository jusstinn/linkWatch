package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations applies pending database migrations
func RunMigrations(db *sql.DB, migrationsDir string) error {
	if err := createMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	migrationFiles, err := findMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to find migration files: %w", err)
	}

	for _, filename := range migrationFiles {
		migrationName := strings.TrimSuffix(filename, ".sql")

		if hasBeenApplied(db, migrationName) {
			continue
		}

		if err := applyMigration(db, migrationsDir, filename, migrationName); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migrationName, err)
		}

		fmt.Printf("Applied migration: %s\n", migrationName)
	}

	return nil
}

func createMigrationsTable(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`

	_, err := db.Exec(query)
	return err
}

func findMigrationFiles(migrationsDir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)

	return files, nil
}

func hasBeenApplied(db *sql.DB, migrationName string) bool {
	var count int
	query := "SELECT COUNT(*) FROM schema_migrations WHERE version = ?"
	err := db.QueryRow(query, migrationName).Scan(&count)

	return err == nil && count > 0
}

func applyMigration(db *sql.DB, migrationsDir, filename, migrationName string) error {
	filePath := filepath.Join(migrationsDir, filename)
	sqlContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(string(sqlContent))
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	_, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", migrationName)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}
