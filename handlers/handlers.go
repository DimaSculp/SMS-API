package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"sms-api-service/config"
	"sms-api-service/database"
	"sms-api-service/types"
)

var (
	getNumberRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.GetNumberRequest{}
		},
	}

	finishActivationRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.FinishActivationRequest{}
		},
	}

	pushSMSRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.PushSMSRequest{}
		},
	}

	getServicesResponsePool = sync.Pool{
		New: func() interface{} {
			return &types.GetServicesResponse{}
		},
	}

	getNumberResponsePool = sync.Pool{
		New: func() interface{} {
			return &types.GetNumberResponse{}
		},
	}

	baseResponsePool = sync.Pool{
		New: func() interface{} {
			return &types.BaseResponse{}
		},
	}

	countryListSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]types.CountryList, 0, 50)
		},
	}

	stringBuilderPool = sync.Pool{
		New: func() interface{} {
			return &strings.Builder{}
		},
	}

	cachedResponses = struct {
		noNumbers1         []byte
		noNumbers2         []byte
		invalidService     []byte
		dbError            []byte
		invalidRequest     []byte
		activationNotFound []byte
		success            []byte
	}{
		noNumbers1:         []byte(`{"status":"NO_NUMBERS1"}`),
		noNumbers2:         []byte(`{"status":"NO_NUMBERS2"}`),
		invalidService:     []byte(`{"status":"INVALID_SERVICE"}`),
		dbError:            []byte(`{"status":"DATABASE_ERROR"}`),
		invalidRequest:     []byte(`{"status":"INVALID_REQUEST"}`),
		activationNotFound: []byte(`{"status":"ACTIVATION_NOT_FOUND"}`),
		success:            []byte(`{"status":"SUCCESS"}`),
	}

	jsonContentType = "application/json; charset=utf-8"
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
		h.sendCachedResponse(w, cachedResponses.dbError)
		return
	}

	countryList := countryListSlicePool.Get().([]types.CountryList)
	countryList = countryList[:0]

	defer func() {
		for i := range countryList {
			countryList[i] = types.CountryList{}
		}
		countryList = countryList[:0]
		countryListSlicePool.Put(countryList)
	}()

	for country, operators := range countryMap {
		cl := types.CountryList{
			Country:     country,
			OperatorMap: operators,
		}
		countryList = append(countryList, cl)
	}

	response := getServicesResponsePool.Get().(*types.GetServicesResponse)
	defer func() {
		*response = types.GetServicesResponse{}
		getServicesResponsePool.Put(response)
	}()

	response.BaseResponse.Status = "SUCCESS"
	response.CountryList = countryList

	h.SendJSONResponse(w, response)
}

func (h *Handler) HandleGetNumber(w http.ResponseWriter, r *http.Request) {
	req := getNumberRequestPool.Get().(*types.GetNumberRequest)
	defer func() {
		*req = types.GetNumberRequest{}
		getNumberRequestPool.Put(req)
	}()

	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		h.sendCachedResponse(w, cachedResponses.invalidRequest)
		return
	}

	phoneNumber, err := database.GetAvailableNumber(h.db, req.Country, req.Operator)
	if err != nil {
		h.sendCachedResponse(w, cachedResponses.noNumbers1)
		return
	}
	defer database.ReturnPhoneNumber(phoneNumber)

	if len(req.ExceptionPhoneSet) > 0 {
		sb := stringBuilderPool.Get().(*strings.Builder)
		defer func() {
			sb.Reset()
			stringBuilderPool.Put(sb)
		}()

		sb.WriteString(strconv.FormatUint(phoneNumber.Number, 10))
		numberStr := sb.String()

		for _, prefix := range req.ExceptionPhoneSet {
			if strings.HasPrefix(numberStr, prefix) {
				h.sendCachedResponse(w, cachedResponses.noNumbers2)
				return
			}
		}
	}

	service, err := database.GetServiceByCode(h.db, req.Service)
	if err != nil {
		h.sendCachedResponse(w, cachedResponses.invalidService)
		return
	}
	defer database.ReturnService(service)

	activationID, err := database.CreateActivation(h.db, phoneNumber.ID, service.ID, req.Sum)
	if err != nil {
		h.sendCachedResponse(w, cachedResponses.dbError)
		return
	}

	go func() {
		if err := database.SetNumberAvailable(h.db, phoneNumber.ID, false); err != nil {
			log.Printf("Failed to mark number as unavailable: %v", err)
		}
	}()

	response := getNumberResponsePool.Get().(*types.GetNumberResponse)
	defer func() {
		*response = types.GetNumberResponse{}
		getNumberResponsePool.Put(response)
	}()

	response.BaseResponse.Status = "SUCCESS"
	response.Number = phoneNumber.Number
	response.ActivationId = activationID
	response.Flashcall = true
	response.Voice = false

	h.SendJSONResponse(w, response)
}

func (h *Handler) HandleFinishActivation(w http.ResponseWriter, r *http.Request) {
	req := finishActivationRequestPool.Get().(*types.FinishActivationRequest)
	defer func() {
		*req = types.FinishActivationRequest{}
		finishActivationRequestPool.Put(req)
	}()

	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		h.sendCachedResponse(w, cachedResponses.invalidRequest)
		return
	}

	err := database.UpdateActivationStatus(h.db, req.ActivationId, req.Status)
	if err != nil {
		h.sendCachedResponse(w, cachedResponses.dbError)
		return
	}

	if req.Status == 3 {
		go func() {
			if err := database.MakeNumberAvailableByActivation(h.db, req.ActivationId); err != nil {
				log.Printf("Failed to mark number as available: %v", err)
			}
		}()
	}

	h.sendCachedResponse(w, cachedResponses.success)
}

func (h *Handler) HandlePushSMS(w http.ResponseWriter, r *http.Request) {
	req := pushSMSRequestPool.Get().(*types.PushSMSRequest)
	defer func() {
		*req = types.PushSMSRequest{}
		pushSMSRequestPool.Put(req)
	}()

	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		h.sendCachedResponse(w, cachedResponses.invalidRequest)
		return
	}

	exists, err := database.CheckActivationExists(h.db, req.ActivationId)
	if err != nil || !exists {
		h.sendCachedResponse(w, cachedResponses.activationNotFound)
		return
	}

	go func() {
		if err := database.StoreSMS(h.db, req.ActivationId, req.SMS); err != nil {
			log.Printf("Failed to store SMS: %v", err)
		}
	}()

	h.sendCachedResponse(w, cachedResponses.success)
}

func (h *Handler) sendCachedResponse(w http.ResponseWriter, response []byte) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (h *Handler) SendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) SendErrorResponse(w http.ResponseWriter, status, message string) {
	switch status {
	case "DATABASE_ERROR":
		h.sendCachedResponse(w, cachedResponses.dbError)
		return
	case "INVALID_REQUEST":
		h.sendCachedResponse(w, cachedResponses.invalidRequest)
		return
	case "INVALID_SERVICE":
		h.sendCachedResponse(w, cachedResponses.invalidService)
		return
	case "ACTIVATION_NOT_FOUND":
		h.sendCachedResponse(w, cachedResponses.activationNotFound)
		return
	}

	response := baseResponsePool.Get().(*types.BaseResponse)
	defer func() {
		*response = types.BaseResponse{}
		baseResponsePool.Put(response)
	}()

	response.Status = status
	h.SendJSONResponse(w, response)
}
