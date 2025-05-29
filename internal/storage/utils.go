package storage

import (
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/ignatij/goflow/internal/log"
)

func InitStore(dbConnStr string) (*PostgresStore, error) {
	store, err := NewPostgresStore(dbConnStr)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func InitStoreAndRunMigrations(dbConnStr string) (*PostgresStore, error) {
	store, err := NewPostgresStore(dbConnStr)
	if err != nil {
		return nil, err
	}
	if err := runMigrations(dbConnStr); err != nil {
		return nil, err
	}
	return store, nil
}

func runMigrations(dbConnStr string) error {
	m, err := migrate.New("file://migrations", dbConnStr)
	if err != nil {
		log.GetLogger().Errorf("Failed to initialize migrations: %v\n", err)
		return err
	}
	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.GetLogger().Info("No DB migrations needed")
			return nil
		}
		log.GetLogger().Errorf("Failed to apply migrations: %v\n", err)
		return err
	}
	log.GetLogger().Info("Migrations applied successfully")
	return nil
}
