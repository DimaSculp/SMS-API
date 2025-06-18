package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"sms-api-service/models"
)

var (
	dbMutex sync.Mutex
)

func Init(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_timeout=30000&_synchronous=NORMAL&_cache_size=1000&_busy_timeout=30000", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, err
	}

	return db, nil
}

func executeWithRetry(db *sql.DB, query string, args ...interface{}) error {
	maxRetries := 5
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		_, err := db.ExecContext(ctx, query, args...)
		cancel()

		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") ||
			strings.Contains(err.Error(), "database table is locked") {

			if attempt < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(attempt))
				log.Printf("Database locked, retrying in %v (attempt %d/%d)", delay, attempt+1, maxRetries)
				time.Sleep(delay)
				continue
			}
		}

		return fmt.Errorf("database operation failed: %w", err)
	}

	return fmt.Errorf("database locked after %d attempts", maxRetries)
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS countries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS phone_numbers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		number INTEGER UNIQUE NOT NULL,
		country_id INTEGER NOT NULL,
		operator TEXT NOT NULL,
		available BOOLEAN DEFAULT TRUE,
		FOREIGN KEY (country_id) REFERENCES countries (id)
	);

	CREATE TABLE IF NOT EXISTS activations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		number_id INTEGER NOT NULL,
		service_id INTEGER NOT NULL,
		status INTEGER DEFAULT 0,
		sum REAL NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		finished_at DATETIME,
		FOREIGN KEY (number_id) REFERENCES phone_numbers (id),
		FOREIGN KEY (service_id) REFERENCES services (id)
	);

	CREATE TABLE IF NOT EXISTS sms_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		activation_id INTEGER NOT NULL,
		text TEXT NOT NULL,
		received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (activation_id) REFERENCES activations (id)
	);
	`

	return executeWithRetry(db, schema)
}

func Seed(db *sql.DB) error {
	// Используем мьютекс для предотвращения конкурентного доступа
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if err := seedCountries(db); err != nil {
		return err
	}

	if err := seedServices(db); err != nil {
		return err
	}

	if err := generateTestNumbers(db); err != nil {
		return err
	}
	log.Print("Data seeded successfully")
	return nil
}

func seedCountries(db *sql.DB) error {
	countries := []models.Country{
		{Code: "rus", Name: "Russia"},
		{Code: "uzb", Name: "Uzbekistan"},
		{Code: "bel", Name: "Belarus"},
	}

	for _, country := range countries {
		err := executeWithRetry(db, "INSERT OR IGNORE INTO countries (code, name) VALUES (?, ?)",
			country.Code, country.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func seedServices(db *sql.DB) error {
	services := []models.Service{
		{Code: "vk", Name: "VKontakte"},
		{Code: "ok", Name: "Odnoklassniki"},
		{Code: "wa", Name: "WhatsApp"},
		{Code: "tg", Name: "Telegram"},
		{Code: "fb", Name: "Facebook"},
	}

	for _, service := range services {
		err := executeWithRetry(db, "INSERT OR IGNORE INTO services (code, name) VALUES (?, ?)",
			service.Code, service.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateTestNumbers(db *sql.DB) error {
	rand.Seed(time.Now().UnixNano())

	// Используем контекст с таймаутом для запроса
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT id, code FROM countries")
	if err != nil {
		return err
	}
	defer rows.Close()

	countryPrefixes := map[int]uint64{
		1: 7,   // Russia
		2: 998, // Uzbekistan
		3: 375, // Belarus
	}

	countries := make([]struct {
		ID   int
		Code string
	}, 0)

	for rows.Next() {
		var countryID int
		var countryCode string
		if err := rows.Scan(&countryID, &countryCode); err != nil {
			continue
		}
		countries = append(countries, struct {
			ID   int
			Code string
		}{countryID, countryCode})
	}
	rows.Close()

	for _, country := range countries {
		prefix, exists := countryPrefixes[country.ID]
		if !exists {
			continue
		}

		numCount := rand.Intn(11) + 20

		batchSize := 10
		for i := 0; i < numCount; i += batchSize {
			end := i + batchSize
			if end > numCount {
				end = numCount
			}

			tx, err := db.Begin()
			if err != nil {
				log.Printf("Error starting transaction: %v", err)
				continue
			}

			for j := i; j < end; j++ {
				var number uint64
				if prefix == 7 { // Russia
					number = 79000000000 + uint64(rand.Intn(999999999))
				} else if prefix == 998 { // Uzbekistan
					number = 998000000000 + uint64(rand.Intn(999999999))
				} else { // Belarus
					number = 375000000000 + uint64(rand.Intn(999999999))
				}

				operator := "any"
				_, err := tx.Exec(`INSERT OR IGNORE INTO phone_numbers 
					(number, country_id, operator, available) VALUES (?, ?, ?, ?)`,
					number, country.ID, operator, true)
				if err != nil {
					log.Printf("Error inserting number in transaction: %v", err)
				}
			}

			maxRetries := 3
			var commitErr error
			for attempt := 0; attempt < maxRetries; attempt++ {
				commitErr = tx.Commit()
				if commitErr == nil {
					break
				}

				if strings.Contains(commitErr.Error(), "database is locked") {
					time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
					continue
				}
				break
			}

			if commitErr != nil {
				tx.Rollback()
				log.Printf("Error committing transaction: %v", commitErr)
			}
		}
	}

	return nil
}
