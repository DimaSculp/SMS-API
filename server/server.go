package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	cfg "sms-api-service/config"
	"sms-api-service/handlers"
	"sms-api-service/types"
)

var (
	bytesBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}

	baseRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.BaseRequest{}
		},
	}

	errorResponses = map[string][]byte{
		"INVALID_REQUEST": []byte(`{"status":"INVALID_REQUEST"}`),
		"INVALID_KEY":     []byte(`{"status":"INVALID_KEY"}`),
		"INVALID_ACTION":  []byte(`{"status":"INVALID_ACTION"}`),
	}

	jsonContentType = []byte("application/json; charset=utf-8")
)

type Server struct {
	db      *sql.DB
	config  cfg.Config
	handler *handlers.Handler
	apiKey  []byte
}

func New(db *sql.DB, config cfg.Config) *Server {
	return &Server{
		db:      db,
		config:  config,
		handler: handlers.New(db, config),
		apiKey:  []byte(config.APIKey),
	}
}

type Handler interface {
	HandleAPIRequest(w http.ResponseWriter, r *http.Request)
}

func (s *Server) HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	buf := bytesBufferPool.Get().([]byte)
	defer func() {
		buf = buf[:0]
		bytesBufferPool.Put(buf)
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendErrorResponseFast(w, "INVALID_REQUEST")
		return
	}

	baseReq := baseRequestPool.Get().(*types.BaseRequest)
	defer func() {
		*baseReq = types.BaseRequest{}
		baseRequestPool.Put(baseReq)
	}()

	if err := json.Unmarshal(body, baseReq); err != nil {
		s.sendErrorResponseFast(w, "INVALID_REQUEST")
		return
	}

	if !bytes.Equal([]byte(baseReq.Key), s.apiKey) {
		s.sendErrorResponseFast(w, "INVALID_KEY")
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	switch baseReq.Action {
	case "GET_NUMBER":
		s.handler.HandleGetNumber(w, r)
	case "PUSH_SMS":
		s.handler.HandlePushSMS(w, r)
	case "FINISH_ACTIVATION":
		s.handler.HandleFinishActivation(w, r)
	case "GET_SERVICES":
		s.handler.HandleGetServices(w)
	default:
		s.sendErrorResponseFast(w, "INVALID_ACTION")
	}
}

func (s *Server) sendErrorResponseFast(w http.ResponseWriter, errorType string) {
	w.Header().Set("Content-Type", string(jsonContentType))
	w.WriteHeader(http.StatusOK)
	w.Write(errorResponses[errorType])
}

func (s *Server) SendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", string(jsonContentType))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) SendErrorResponse(w http.ResponseWriter, status, message string) {
	if cachedResponse, exists := errorResponses[status]; exists {
		w.Header().Set("Content-Type", string(jsonContentType))
		w.WriteHeader(http.StatusOK)
		w.Write(cachedResponse)
		return
	}

	response := types.BaseResponse{Status: status}
	s.SendJSONResponse(w, response)
}
