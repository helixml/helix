package errutil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// CloudError represents a structured error response
type CloudError struct {
	StatusCode int    `json:"status_code"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *CloudError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error codes
const (
	CloudErrorCodeInternalServerError        = "INTERNAL_SERVER_ERROR"
	CloudErrorCodeInvalidParameter          = "INVALID_PARAMETER"
	CloudErrorCodeDeviceConnectionFailure   = "DEVICE_CONNECTION_FAILURE"
)

// NewCloudError creates a new CloudError
func NewCloudError(statusCode int, code, message string) *CloudError {
	return &CloudError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

// WriteCloudError writes a CloudError as JSON response
func WriteCloudError(w http.ResponseWriter, err *CloudError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	json.NewEncoder(w).Encode(err)
}