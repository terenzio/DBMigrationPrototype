package repository

import (
	"database/sql"
	"fmt"

	_ "github.com/godror/godror"

	"github.com/terence/dbmigration-backend/internal/config"
)

func NewOracleDB(cfg config.DBConfig) (*sql.DB, error) {
	db, err := sql.Open("godror", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open oracle connection: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping oracle: %w", err)
	}
	return db, nil
}
