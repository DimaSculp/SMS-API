package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"sms-api-service/config"
	"sms-api-service/database"
	"sms-api-service/types"
)

type Handler struct {
	db     *sql.DB
	config config.Config
}

func New(db *sql.DB, cfg config.Config) *Handler {
	return &Handler{
		db:     db,
		config: cfg,
	}
}

func (h *Handler) HandleGetServices(w http.ResponseWriter) {
	countryMap, err := database.GetAvailableServices(h.db)
	if err != nil {
		h.SendErrorResponse(w, "DATABASE_ERROR", "Database query failed")
		return
	}

	var countryList []types.CountryList
	for country, operators := range countryMap {
		cl := types.CountryList{
			Country:     country,
			OperatorMap: operators,
		}
		countryList = append(countryList, cl)
	}

	response := types.GetServicesResponse{
		BaseResponse: types.BaseResponse{Status: "SUCCESS"},
		CountryList:  countryList,
	}

	h.SendJSONResponse(w, response)
}

func (h *Handler) HandleGetNumber(w http.ResponseWriter, r *http.Request) {
	var req types.GetNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.SendErrorResponse(w, "INVALID_REQUEST", "Invalid request format")
		return
	}

	phoneNumber, err := database.GetAvailableNumber(h.db, req.Country, req.Operator)
	if err != nil {
		response := types.GetNumberResponse{
			BaseResponse: types.BaseResponse{Status: "NO_NUMBERS1"},
		}
		h.SendJSONResponse(w, response)
		return
	}

	numberStr := strconv.FormatUint(phoneNumber.Number, 10)
	for _, prefix := range req.ExceptionPhoneSet {
		if strings.HasPrefix(numberStr, prefix) {
			response := types.GetNumberResponse{
				BaseResponse: types.BaseResponse{Status: "NO_NUMBERS2"},
			}
			h.SendJSONResponse(w, response)
			return
		}
	}

	service, err := database.GetServiceByCode(h.db, req.Service)
	if err != nil {
		h.SendErrorResponse(w, "INVALID_SERVICE", "Service not found")
		return
	}

	activationID, err := database.CreateActivation(h.db, phoneNumber.ID, service.ID, req.Sum)
	if err != nil {
		h.SendErrorResponse(w, "DATABASE_ERROR", "Failed to create activation")
		return
	}

	err = database.SetNumberAvailable(h.db, phoneNumber.ID, false)
	if err != nil {
		log.Printf("Failed to mark number as unavailable: %v", err)
	}

	response := types.GetNumberResponse{
		BaseResponse: types.BaseResponse{Status: "SUCCESS"},
		Number:       phoneNumber.Number,
		ActivationId: activationID,
		Flashcall:    true,
		Voice:        false,
	}

	h.SendJSONResponse(w, response)
}

func (h *Handler) HandleFinishActivation(w http.ResponseWriter, r *http.Request) {
	var req types.FinishActivationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.SendErrorResponse(w, "INVALID_REQUEST", "Invalid request format")
		return
	}

	err := database.UpdateActivationStatus(h.db, req.ActivationId, req.Status)
	if err != nil {
		h.SendErrorResponse(w, "DATABASE_ERROR", "Failed to update activation")
		return
	}

	if req.Status == 3 {
		err = database.MakeNumberAvailableByActivation(h.db, req.ActivationId)
		if err != nil {
			log.Printf("Failed to mark number as available: %v", err)
		}
	}

	response := types.BaseResponse{Status: "SUCCESS"}
	h.SendJSONResponse(w, response)
}

func (h *Handler) HandlePushSMS(w http.ResponseWriter, r *http.Request) {
	var req types.PushSMSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.SendErrorResponse(w, "INVALID_REQUEST", "Invalid request format")
		return
	}

	exists, err := database.CheckActivationExists(h.db, req.ActivationId)
	if err != nil || !exists {
		h.SendErrorResponse(w, "ACTIVATION_NOT_FOUND", "Activation not found")
		return
	}

	err = database.StoreSMS(h.db, req.ActivationId, req.SMS)
	if err != nil {
		h.SendErrorResponse(w, "DATABASE_ERROR", "Failed to store SMS")
		return
	}

	response := types.BaseResponse{Status: "SUCCESS"}
	h.SendJSONResponse(w, response)
}

func (h *Handler) SendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) SendErrorResponse(w http.ResponseWriter, status, message string) {
	response := types.BaseResponse{Status: status}
	h.SendJSONResponse(w, response)
}
