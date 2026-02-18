package main

import (
	"log"
	"os"

	"github.com/terence/dbmigration-backend/internal/config"
	"github.com/terence/dbmigration-backend/internal/handler"
	"github.com/terence/dbmigration-backend/internal/repository"
	"github.com/terence/dbmigration-backend/internal/router"
	"github.com/terence/dbmigration-backend/internal/service"
)

func main() {
	cfg := config.LoadDBConfig()

	db, err := repository.NewOracleDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Oracle: %v", err)
	}
	defer db.Close()

	repo := repository.NewProductSuiteRepository(db)
	svc := service.NewProductSuiteService(repo)
	hdl := handler.NewProductSuiteHandler(svc)

	r := router.Setup(hdl, db)

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
