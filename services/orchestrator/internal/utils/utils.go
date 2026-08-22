package utils

import (
	"encoding/json"
	"net/http"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
)

type Envelope map[string]any

func WriteJson(w http.ResponseWriter, status int, response *dtos.HTTPResponse) error {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(*response)
	return nil

}

func WriteJsonHealthCheck(w http.ResponseWriter, status int, response interface{}) error {
	js, err := json.MarshalIndent(response, "", "")

	if err != nil {
		return err
	}

	js = append(js, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)
	return nil
}

func writeResponse(isSucessful bool, data interface{}, err, message string, meta *dtos.PaginationMeta) *dtos.HTTPResponse {
	return &dtos.HTTPResponse{
		Success: isSucessful,
		Data:    data,
		Error:   err,
		Message: message,
		Meta:    meta,
	}
}

func WriteResponseSuccess(data interface{}, err, message string, meta *dtos.PaginationMeta) *dtos.HTTPResponse {
	return writeResponse(true, data, err, message, meta)
}

func WriteResponseFailed(data interface{}, err, message string, meta *dtos.PaginationMeta) *dtos.HTTPResponse {
	return writeResponse(false, data, err, message, meta)
}
