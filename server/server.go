package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	cfg "sms-api-service/config"
	"sms-api-service/handlers"
	"sms-api-service/types"
)

type Server struct {
	db      *sql.DB
	config  cfg.Config
	handler *handlers.Handler
}

func New(db *sql.DB, config cfg.Config) *Server {
	return &Server{
		db:      db,
		config:  config,
		handler: handlers.New(db, config),
	}
}

type Handler interface {
	HandleAPIRequest(w http.ResponseWriter, r *http.Request)
}

func (s *Server) HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.SendErrorResponse(w, "INVALID_REQUEST", "Cannot read request body")
		return
	}

	var baseReq types.BaseRequest
	if err := json.Unmarshal(body, &baseReq); err != nil {
		s.SendErrorResponse(w, "INVALID_REQUEST", "Invalid JSON")
		return
	}

	if baseReq.Key != s.config.APIKey {
		s.SendErrorResponse(w, "INVALID_KEY", "Invalid API key")
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	switch baseReq.Action {
	case "GET_SERVICES":
		s.handler.HandleGetServices(w)
	case "GET_NUMBER":
		s.handler.HandleGetNumber(w, r)
	case "FINISH_ACTIVATION":
		s.handler.HandleFinishActivation(w, r)
	case "PUSH_SMS":
		s.handler.HandlePushSMS(w, r)

	default:
		s.SendErrorResponse(w, "INVALID_ACTION", "Unknown action")
	}
}

func (s *Server) SendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) SendErrorResponse(w http.ResponseWriter, status, message string) {
	response := types.BaseResponse{Status: status}
	s.SendJSONResponse(w, response)
}
