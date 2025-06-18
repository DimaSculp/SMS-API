package main

import (
	"log"
	"net/http"

	"sms-api-service/config"
	"sms-api-service/database"
	"sms-api-service/server"
)

func main() {
	cfg := config.Load()

	db, err := database.Init(cfg.DBPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	if err := database.Seed(db); err != nil {
		log.Fatal("Failed to seed data:", err)
	}

	srv := server.New(db, cfg)
	http.HandleFunc("/GrizzlySMSbyDima.php", srv.HandleAPIRequest)

	log.Printf("SMS API Service starting on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
