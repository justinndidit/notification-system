package utils

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
)

func MapToDTO(body *dtos.NotificationRequest, cID string) *dtos.NotificationRequestDTO {

	return &dtos.NotificationRequestDTO{
		NotificationType: body.NotificationType,
		UserID:           body.UserID,
		TemplateCode:     body.TemplateCode,
		Variables:        body.Variables,
		RequestID:        body.RequestID,
		Priority:         body.Priority,
		MetaData:         body.MetaData,
		CorrelationID:    cID,
	}
}

func ToNotificationModel(dto *dtos.NotificationRequestDTO) *models.Notification {

	return &models.Notification{
		UserID:        uuid.MustParse(dto.UserID),
		TemplateID:    uuid.MustParse(dto.TemplateCode),
		CorrelationID: uuid.MustParse(dto.CorrelationID),
		Channel:       dto.NotificationType,
		Priority:      dtos.NotificationPriorityToString(dtos.NotificationPriority(dto.Priority)),
		Variables:     dto.Variables,
	}
}

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
