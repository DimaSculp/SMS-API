package database

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"sms-api-service/models"
)

var (
	phoneNumberPool = sync.Pool{
		New: func() interface{} {
			return &models.PhoneNumber{}
		},
	}

	servicePool = sync.Pool{
		New: func() interface{} {
			return &models.Service{}
		},
	}

	activationPool = sync.Pool{
		New: func() interface{} {
			return &models.Activation{}
		},
	}

	smsSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]models.SMS, 0, 10)
		},
	}

	preparedQueries = struct {
		getAvailableServices   string
		getAvailableNumber     string
		getServiceByCode       string
		createActivation       string
		setNumberAvailable     string
		updateActivationStatus string
		makeNumberAvailable    string
		checkActivationExists  string
		storeSMS               string
		getActivationByID      string
		getSMSByActivation     string
	}{
		getAvailableServices: `
			SELECT c.code, pn.operator, srv.code, COUNT(*)
			FROM phone_numbers pn
			JOIN countries c ON pn.country_id = c.id
			CROSS JOIN services srv
			WHERE pn.available = 1
			GROUP BY c.code, pn.operator, srv.code
			HAVING COUNT(*) > 0`,

		getAvailableNumber: `
			SELECT pn.id, pn.number 
			FROM phone_numbers pn
			JOIN countries c ON pn.country_id = c.id
			WHERE c.code = ? AND pn.operator = ? AND pn.available = 1
			ORDER BY RANDOM()
			LIMIT 1`,

		getServiceByCode: `SELECT id, code, name FROM services WHERE code = ?`,

		createActivation: `
			INSERT INTO activations (number_id, service_id, sum, created_at)
			VALUES (?, ?, ?, ?)`,

		setNumberAvailable: `UPDATE phone_numbers SET available = ? WHERE id = ?`,

		updateActivationStatus: `
			UPDATE activations 
			SET status = ?, finished_at = ?
			WHERE id = ?`,

		makeNumberAvailable: `
			UPDATE phone_numbers 
			SET available = 1 
			WHERE id = (SELECT number_id FROM activations WHERE id = ?)`,

		checkActivationExists: `SELECT 1 FROM activations WHERE id = ? LIMIT 1`,

		storeSMS: `
			INSERT INTO sms_messages (activation_id, text, received_at)
			VALUES (?, ?, ?)`,

		getActivationByID: `
			SELECT id, number_id, service_id, status, sum, created_at, finished_at
			FROM activations WHERE id = ?`,

		getSMSByActivation: `
			SELECT id, activation_id, text, received_at
			FROM sms_messages 
			WHERE activation_id = ?
			ORDER BY received_at ASC`,
	}
)

var serviceCache = struct {
	sync.RWMutex
	services map[string]*models.Service
}{
	services: make(map[string]*models.Service),
}

func GetAvailableServices(db *sql.DB) (map[string]map[string]map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, preparedQueries.getAvailableServices)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	countryMap := make(map[string]map[string]map[string]int, 50)

	for rows.Next() {
		var country, operator, service string
		var count int

		if err := rows.Scan(&country, &operator, &service, &count); err != nil {
			continue
		}

		if countryMap[country] == nil {
			countryMap[country] = make(map[string]map[string]int, 10)
		}
		if countryMap[country][operator] == nil {
			countryMap[country][operator] = make(map[string]int, 20)
		}

		countryMap[country][operator][service] = count
	}

	return countryMap, rows.Err()
}

func GetAvailableNumber(db *sql.DB, country, operator string) (*models.PhoneNumber, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	phoneNumber := phoneNumberPool.Get().(*models.PhoneNumber)

	err := db.QueryRowContext(ctx, preparedQueries.getAvailableNumber, country, operator).
		Scan(&phoneNumber.ID, &phoneNumber.Number)
	if err != nil {
		*phoneNumber = models.PhoneNumber{}
		phoneNumberPool.Put(phoneNumber)
		return nil, err
	}

	return phoneNumber, nil
}

func ReturnPhoneNumber(phoneNumber *models.PhoneNumber) {
	if phoneNumber != nil {
		*phoneNumber = models.PhoneNumber{}
		phoneNumberPool.Put(phoneNumber)
	}
}

func GetServiceByCode(db *sql.DB, serviceCode string) (*models.Service, error) {
	serviceCache.RLock()
	if cachedService, exists := serviceCache.services[serviceCode]; exists {
		serviceCache.RUnlock()
		service := servicePool.Get().(*models.Service)
		*service = *cachedService
		return service, nil
	}
	serviceCache.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	service := servicePool.Get().(*models.Service)
	err := db.QueryRowContext(ctx, preparedQueries.getServiceByCode, serviceCode).
		Scan(&service.ID, &service.Code, &service.Name)
	if err != nil {
		*service = models.Service{}
		servicePool.Put(service)
		return nil, err
	}

	serviceCache.Lock()
	serviceCache.services[serviceCode] = &models.Service{
		ID:   service.ID,
		Code: service.Code,
		Name: service.Name,
	}
	serviceCache.Unlock()

	return service, nil
}

func ReturnService(service *models.Service) {
	if service != nil {
		*service = models.Service{}
		servicePool.Put(service)
	}
}

func CreateActivation(db *sql.DB, numberID, serviceID int, sum float64) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := db.ExecContext(ctx, preparedQueries.createActivation,
		numberID, serviceID, sum, time.Now())
	if err != nil {
		return 0, err
	}

	activationID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return uint64(activationID), nil
}

func SetNumberAvailable(db *sql.DB, numberID int, available bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, preparedQueries.setNumberAvailable, available, numberID)
	return err
}

func UpdateActivationStatus(db *sql.DB, activationID uint64, status int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := db.ExecContext(ctx, preparedQueries.updateActivationStatus,
		status, time.Now(), activationID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func MakeNumberAvailableByActivation(db *sql.DB, activationID uint64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, preparedQueries.makeNumberAvailable, activationID)
	return err
}

func CheckActivationExists(db *sql.DB, activationID uint64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var exists int
	err := db.QueryRowContext(ctx, preparedQueries.checkActivationExists, activationID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func StoreSMS(db *sql.DB, activationID uint64, smsText string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, preparedQueries.storeSMS,
		activationID, smsText, time.Now())
	return err
}

func GetActivationByID(db *sql.DB, activationID uint64) (*models.Activation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	activation := activationPool.Get().(*models.Activation)

	err := db.QueryRowContext(ctx, preparedQueries.getActivationByID, activationID).Scan(
		&activation.ID,
		&activation.NumberID,
		&activation.ServiceID,
		&activation.Status,
		&activation.Sum,
		&activation.CreatedAt,
		&activation.FinishedAt,
	)
	if err != nil {
		*activation = models.Activation{}
		activationPool.Put(activation)
		return nil, err
	}

	return activation, nil
}

func ReturnActivation(activation *models.Activation) {
	if activation != nil {
		*activation = models.Activation{}
		activationPool.Put(activation)
	}
}

func GetSMSByActivation(db *sql.DB, activationID uint64) ([]models.SMS, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, preparedQueries.getSMSByActivation, activationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := smsSlicePool.Get().([]models.SMS)
	messages = messages[:0]

	for rows.Next() {
		var sms models.SMS
		if err := rows.Scan(&sms.ID, &sms.ActivationID, &sms.Text, &sms.ReceivedAt); err != nil {
			continue
		}
		messages = append(messages, sms)
	}

	if err := rows.Err(); err != nil {
		messages = messages[:0]
		smsSlicePool.Put(messages)
		return nil, err
	}

	result := make([]models.SMS, len(messages))
	copy(result, messages)

	messages = messages[:0]
	smsSlicePool.Put(messages)

	return result, nil
}

func ClearServiceCache() {
	serviceCache.Lock()
	defer serviceCache.Unlock()

	for k := range serviceCache.services {
		delete(serviceCache.services, k)
	}
}
