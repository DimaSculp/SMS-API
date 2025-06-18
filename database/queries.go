package database

import (
	"database/sql"
	"time"

	"sms-api-service/models"
)

func GetAvailableServices(db *sql.DB) (map[string]map[string]map[string]int, error) {
	query := `
		SELECT c.code as country, pn.operator, srv.code as service, COUNT(*) as count
		FROM phone_numbers pn
		JOIN countries c ON pn.country_id = c.id
		CROSS JOIN services srv
		WHERE pn.available = TRUE
		GROUP BY c.code, pn.operator, srv.code
		HAVING count > 0
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	countryMap := make(map[string]map[string]map[string]int)

	for rows.Next() {
		var country, operator, service string
		var count int

		if err := rows.Scan(&country, &operator, &service, &count); err != nil {
			continue
		}

		if countryMap[country] == nil {
			countryMap[country] = make(map[string]map[string]int)
		}
		if countryMap[country][operator] == nil {
			countryMap[country][operator] = make(map[string]int)
		}

		countryMap[country][operator][service] = count
	}

	return countryMap, nil
}

func GetAvailableNumber(db *sql.DB, country, operator string) (*models.PhoneNumber, error) {
	query := `
		SELECT pn.id, pn.number 
		FROM phone_numbers pn
		JOIN countries c ON pn.country_id = c.id
		WHERE c.code = ? AND pn.operator = ? AND pn.available = TRUE
		ORDER BY RANDOM()
		LIMIT 1
	`

	var phoneNumber models.PhoneNumber
	err := db.QueryRow(query, country, operator).Scan(&phoneNumber.ID, &phoneNumber.Number)
	if err != nil {
		return nil, err
	}

	return &phoneNumber, nil
}

func GetServiceByCode(db *sql.DB, serviceCode string) (*models.Service, error) {
	var service models.Service
	err := db.QueryRow("SELECT id, code, name FROM services WHERE code = ?", serviceCode).
		Scan(&service.ID, &service.Code, &service.Name)
	if err != nil {
		return nil, err
	}

	return &service, nil
}

func CreateActivation(db *sql.DB, numberID, serviceID int, sum float64) (uint64, error) {
	result, err := db.Exec(`
		INSERT INTO activations (number_id, service_id, sum, created_at)
		VALUES (?, ?, ?, ?)`,
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
	_, err := db.Exec("UPDATE phone_numbers SET available = ? WHERE id = ?", available, numberID)
	return err
}

func UpdateActivationStatus(db *sql.DB, activationID uint64, status int) error {
	now := time.Now()
	result, err := db.Exec(`
		UPDATE activations 
		SET status = ?, finished_at = ?
		WHERE id = ?`,
		status, now, activationID)
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
	_, err := db.Exec(`
		UPDATE phone_numbers 
		SET available = TRUE 
		WHERE id = (SELECT number_id FROM activations WHERE id = ?)`,
		activationID)
	return err
}

func CheckActivationExists(db *sql.DB, activationID uint64) (bool, error) {
	var exists int
	err := db.QueryRow("SELECT 1 FROM activations WHERE id = ? LIMIT 1", activationID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func StoreSMS(db *sql.DB, activationID uint64, smsText string) error {
	_, err := db.Exec(`
		INSERT INTO sms_messages (activation_id, text, received_at)
		VALUES (?, ?, ?)`,
		activationID, smsText, time.Now())
	return err
}

func GetActivationByID(db *sql.DB, activationID uint64) (*models.Activation, error) {
	var activation models.Activation
	err := db.QueryRow(`
		SELECT id, number_id, service_id, status, sum, created_at, finished_at
		FROM activations WHERE id = ?`,
		activationID).Scan(
		&activation.ID,
		&activation.NumberID,
		&activation.ServiceID,
		&activation.Status,
		&activation.Sum,
		&activation.CreatedAt,
		&activation.FinishedAt,
	)
	if err != nil {
		return nil, err
	}

	return &activation, nil
}

func GetSMSByActivation(db *sql.DB, activationID uint64) ([]models.SMS, error) {
	rows, err := db.Query(`
		SELECT id, activation_id, text, received_at
		FROM sms_messages 
		WHERE activation_id = ?
		ORDER BY received_at ASC`,
		activationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.SMS
	for rows.Next() {
		var sms models.SMS
		if err := rows.Scan(&sms.ID, &sms.ActivationID, &sms.Text, &sms.ReceivedAt); err != nil {
			continue
		}
		messages = append(messages, sms)
	}

	return messages, nil
}
