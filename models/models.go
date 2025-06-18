package models

import "time"

type Country struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type Service struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type PhoneNumber struct {
	ID        int    `json:"id"`
	Number    uint64 `json:"number"`
	CountryID int    `json:"country_id"`
	Operator  string `json:"operator"`
	Available bool   `json:"available"`
}

type Activation struct {
	ID         uint64     `json:"id"`
	NumberID   int        `json:"number_id"`
	ServiceID  int        `json:"service_id"`
	Status     int        `json:"status"`
	Sum        float64    `json:"sum"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type SMS struct {
	ID           int       `json:"id"`
	ActivationID uint64    `json:"activation_id"`
	Text         string    `json:"text"`
	ReceivedAt   time.Time `json:"received_at"`
}
