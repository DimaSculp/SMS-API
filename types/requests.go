package types

import (
	"database/sql"
	"time"
)

type Config struct {
	Port   string
	DBPath string
	APIKey string
}

type Server struct {
	db     *sql.DB
	config Config
}

type BaseRequest struct {
	Action string `json:"action"`
	Key    string `json:"key"`
}

type GetNumberRequest struct {
	BaseRequest
	Country           string   `json:"country"`
	Service           string   `json:"service"`
	Operator          string   `json:"operator"`
	Sum               float64  `json:"sum"`
	ExceptionPhoneSet []string `json:"exceptionPhoneSet,omitempty"`
}

type FinishActivationRequest struct {
	BaseRequest
	ActivationId uint64 `json:"activationId"`
	Status       int    `json:"status"`
}

type PushSMSRequest struct {
	BaseRequest
	ActivationId uint64 `json:"activationId"`
	SMS          string `json:"sms"`
}

type BaseResponse struct {
	Status string `json:"status"`
}

type CountryList struct {
	Country     string                    `json:"country"`
	OperatorMap map[string]map[string]int `json:"operatorMap"`
}

type GetServicesResponse struct {
	BaseResponse
	CountryList []CountryList `json:"countryList"`
}

type GetNumberResponse struct {
	BaseResponse
	Number       uint64 `json:"number,omitempty"`
	ActivationId uint64 `json:"activationId,omitempty"`
	Flashcall    bool   `json:"flashcall,omitempty"`
	Voice        bool   `json:"voice,omitempty"`
}

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
	Status     int        `json:"status"` // 0-active, 3-finished
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
