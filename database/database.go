package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"sms-api-service/models"
)

// DatabaseConfig содержит конфигурацию для подключения к БД
type DatabaseConfig struct {
	Path            string
	JournalMode     string
	Timeout         int
	Synchronous     string
	CacheSize       int
	BusyTimeout     int
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig(dbPath string) *DatabaseConfig {
	return &DatabaseConfig{
		Path:            dbPath,
		JournalMode:     "WAL",
		Timeout:         30000,
		Synchronous:     "NORMAL",
		CacheSize:       1000,
		BusyTimeout:     30000,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
	}
}

// Database представляет обертку над sql.DB с дополнительной функциональностью
type Database struct {
	*sql.DB
	config *DatabaseConfig
}

// Init инициализирует подключение к базе данных
func Init(config *DatabaseConfig) (*Database, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=%s&_timeout=%d&_synchronous=%s&_cache_size=%d&_busy_timeout=%d",
		config.Path, config.JournalMode, config.Timeout, config.Synchronous, config.CacheSize, config.BusyTimeout)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{
		DB:     db,
		config: config,
	}

	if err := database.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return database, nil
}

// Close закрывает подключение к БД
func (d *Database) Close() error {
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}

// ExecuteWithRetry выполняет запрос с повторными попытками при блокировке БД
func (d *Database) ExecuteWithRetry(ctx context.Context, query string, args ...interface{}) error {
	const (
		maxRetries = 5
		baseDelay  = 100 * time.Millisecond
	)

	for attempt := 0; attempt < maxRetries; attempt++ {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)

		_, err := d.ExecContext(ctxWithTimeout, query, args...)
		cancel()

		if err == nil {
			return nil
		}

		if !isRetryableError(err) {
			return fmt.Errorf("database operation failed: %w", err)
		}

		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<uint(attempt))
			log.Printf("Database locked, retrying in %v (attempt %d/%d)", delay, attempt+1, maxRetries)

			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("database locked after %d attempts", maxRetries)
}

// isRetryableError проверяет, является ли ошибка повторяемой
func isRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database table is locked")
}

// createTables создает необходимые таблицы
func (d *Database) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS countries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS phone_numbers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		number INTEGER UNIQUE NOT NULL,
		country_id INTEGER NOT NULL,
		operator TEXT NOT NULL,
		available BOOLEAN DEFAULT TRUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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

	-- Индексы для оптимизации запросов
	CREATE INDEX IF NOT EXISTS idx_phone_numbers_country_available ON phone_numbers(country_id, available);
	CREATE INDEX IF NOT EXISTS idx_activations_status ON activations(status);
	CREATE INDEX IF NOT EXISTS idx_activations_created_at ON activations(created_at);
	CREATE INDEX IF NOT EXISTS idx_sms_messages_activation_id ON sms_messages(activation_id);
	`

	ctx := context.Background()
	return d.ExecuteWithRetry(ctx, schema)
}

// SeedData структура для конфигурации тестовых данных
type SeedData struct {
	Countries    []models.Country
	Services     []models.Service
	NumbersRange struct {
		Min, Max int
	}
}

// DefaultSeedData возвращает данные для заполнения по умолчанию
func DefaultSeedData() *SeedData {
	return &SeedData{
		Countries: []models.Country{
			{Code: "rus", Name: "Russia"},
			{Code: "uzb", Name: "Uzbekistan"},
			{Code: "bel", Name: "Belarus"},
		},
		Services: []models.Service{
			{Code: "vk", Name: "VKontakte"},
			{Code: "ok", Name: "Odnoklassniki"},
			{Code: "wa", Name: "WhatsApp"},
			{Code: "tg", Name: "Telegram"},
			{Code: "fb", Name: "Facebook"},
		},
		NumbersRange: struct{ Min, Max int }{Min: 20, Max: 30},
	}
}

// Seed заполняет базу данных тестовыми данными
func (d *Database) Seed(ctx context.Context, seedData *SeedData) error {
	if err := d.seedCountries(ctx, seedData.Countries); err != nil {
		return fmt.Errorf("failed to seed countries: %w", err)
	}

	if err := d.seedServices(ctx, seedData.Services); err != nil {
		return fmt.Errorf("failed to seed services: %w", err)
	}

	if err := d.generateTestNumbers(ctx, seedData.NumbersRange.Min, seedData.NumbersRange.Max); err != nil {
		return fmt.Errorf("failed to generate test numbers: %w", err)
	}

	log.Print("Data seeded successfully")
	return nil
}

// seedCountries заполняет таблицу стран
func (d *Database) seedCountries(ctx context.Context, countries []models.Country) error {
	for _, country := range countries {
		err := d.ExecuteWithRetry(ctx, "INSERT OR IGNORE INTO countries (code, name) VALUES (?, ?)",
			country.Code, country.Name)
		if err != nil {
			return fmt.Errorf("failed to insert country %s: %w", country.Code, err)
		}
	}
	return nil
}

// seedServices заполняет таблицу сервисов
func (d *Database) seedServices(ctx context.Context, services []models.Service) error {
	for _, service := range services {
		err := d.ExecuteWithRetry(ctx, "INSERT OR IGNORE INTO services (code, name) VALUES (?, ?)",
			service.Code, service.Name)
		if err != nil {
			return fmt.Errorf("failed to insert service %s: %w", service.Code, err)
		}
	}
	return nil
}

// CountryPrefix представляет префикс страны для генерации номеров
type CountryPrefix struct {
	ID     int
	Code   string
	Prefix uint64
}

// getCountryPrefixes возвращает префиксы стран
func getCountryPrefixes() map[string]uint64 {
	return map[string]uint64{
		"rus": 7,
		"uzb": 998,
		"bel": 375,
	}
}

// generateTestNumbers генерирует тестовые номера телефонов
func (d *Database) generateTestNumbers(ctx context.Context, minCount, maxCount int) error {
	countries, err := d.getCountriesWithPrefixes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get countries: %w", err)
	}

	for _, country := range countries {
		numCount := rand.Intn(maxCount-minCount+1) + minCount

		if err := d.insertNumbersBatch(ctx, country, numCount); err != nil {
			return fmt.Errorf("failed to insert numbers for country %s: %w", country.Code, err)
		}
	}

	return nil
}

// getCountriesWithPrefixes получает страны с их префиксами
func (d *Database) getCountriesWithPrefixes(ctx context.Context) ([]CountryPrefix, error) {
	rows, err := d.QueryContext(ctx, "SELECT id, code FROM countries")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefixes := getCountryPrefixes()
	var countries []CountryPrefix

	for rows.Next() {
		var cp CountryPrefix
		if err := rows.Scan(&cp.ID, &cp.Code); err != nil {
			continue
		}

		if prefix, exists := prefixes[cp.Code]; exists {
			cp.Prefix = prefix
			countries = append(countries, cp)
		}
	}

	return countries, rows.Err()
}

// insertNumbersBatch вставляет номера телефонов батчами
func (d *Database) insertNumbersBatch(ctx context.Context, country CountryPrefix, count int) error {
	const batchSize = 100

	for i := 0; i < count; i += batchSize {
		end := i + batchSize
		if end > count {
			end = count
		}

		if err := d.insertNumbersBatchTx(ctx, country, i, end); err != nil {
			return err
		}
	}

	return nil
}

// insertNumbersBatchTx вставляет батч номеров в рамках транзакции
func (d *Database) insertNumbersBatchTx(ctx context.Context, country CountryPrefix, start, end int) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO phone_numbers 
		(number, country_id, operator, available) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for i := start; i < end; i++ {
		number := d.generatePhoneNumber(country.Prefix)

		if _, err := stmt.ExecContext(ctx, number, country.ID, "any", true); err != nil {
			log.Printf("Failed to insert number %d: %v", number, err)
		}
	}

	return tx.Commit()
}

// generatePhoneNumber генерирует номер телефона для заданного префикса
func (d *Database) generatePhoneNumber(prefix uint64) uint64 {
	switch prefix {
	case 7: // Russia
		return 79000000000 + uint64(rand.Intn(999999999))
	case 998: // Uzbekistan
		return 998000000000 + uint64(rand.Intn(999999999))
	case 375: // Belarus
		return 375000000000 + uint64(rand.Intn(999999999))
	default:
		return prefix*1000000000 + uint64(rand.Intn(999999999))
	}
}
