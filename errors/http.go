/*
Copyright (c) Gatis Beikerts

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package errors

import (
	"encoding/json"
	"net/http"
)

// ErrorCode represents a standardized error code
type ErrorCode string

// Standard error codes
const (
	// Authentication errors
	ErrInvalidAPIKey      ErrorCode = "invalid_api_key"
	ErrUnauthorizedAccess ErrorCode = "unauthorized_access"

	// Validation errors
	ErrInvalidDoctype      ErrorCode = "invalid_doctype"
	ErrInvalidCountry      ErrorCode = "invalid_country"
	ErrCountryNotSupported ErrorCode = "country_not_supported"
	ErrInvalidExpiryDate   ErrorCode = "invalid_expiry_date"
	ErrInvalidStatus       ErrorCode = "invalid_status"
	ErrInvalidURI          ErrorCode = "invalid_uri"
	ErrInvalidIndex        ErrorCode = "invalid_index"
	ErrInvalidIdentifier   ErrorCode = "invalid_identifier"
	ErrInvalidAccept       ErrorCode = "invalid_accept_header"
	ErrInvalidPath         ErrorCode = "invalid_path"

	// Business logic errors
	ErrNoIndicesAvailable ErrorCode = "no_indices_available"
	ErrListNotFound       ErrorCode = "list_not_found"
	ErrIndexOutOfRange    ErrorCode = "index_out_of_range"

	// Cryptographic errors
	ErrPrivateKeyLoad     ErrorCode = "private_key_load_failed"
	ErrCertificateLoad    ErrorCode = "certificate_load_failed"
	ErrJWTSigning         ErrorCode = "jwt_signing_failed"
	ErrCWTSigning         ErrorCode = "cwt_signing_failed"
	ErrCWTEncoding        ErrorCode = "cwt_encoding_failed"
	ErrUnsupportedKeyType ErrorCode = "unsupported_key_type"
	ErrEncryptedKey       ErrorCode = "encrypted_key_not_supported"

	// System errors
	ErrInternalServer     ErrorCode = "internal_server_error"
	ErrBadRequest         ErrorCode = "bad_request"
	ErrParseForm          ErrorCode = "parse_form_error"
	ErrStatusUpdateFailed ErrorCode = "status_update_failed"

	// Storage errors
	ErrStorageBackendInvalid   ErrorCode = "storage_backend_invalid"
	ErrStorageConfigMissing    ErrorCode = "storage_config_missing"
	ErrStorageConnectionFailed ErrorCode = "storage_connection_failed"
	ErrStorageOperationFailed  ErrorCode = "storage_operation_failed"
	ErrVersionMismatchCode     ErrorCode = "version_mismatch"
)

// ErrorResponse represents the standardized error response structure
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and message
type ErrorDetail struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Standard error messages
var errorMessages = map[ErrorCode]string{
	// Authentication errors
	ErrInvalidAPIKey:      "Invalid API key provided.",
	ErrUnauthorizedAccess: "Unauthorized access to this resource.",

	// Validation errors
	ErrInvalidDoctype:      "Invalid document type provided.",
	ErrInvalidCountry:      "Invalid country code provided.",
	ErrCountryNotSupported: "Country code not supported for this service.",
	ErrInvalidExpiryDate:   "Invalid expiry date format. Expected YYYY-MM-DD.",
	ErrInvalidStatus:       "Invalid status value provided.",
	ErrInvalidURI:          "Invalid URI format provided.",
	ErrInvalidIndex:        "Invalid index value provided.",
	ErrInvalidIdentifier:   "Invalid identifier provided.",
	ErrInvalidAccept:       "Only application/statuslist+jwt and application/statuslist+cwt are supported in Accept header.",
	ErrInvalidPath:         "Invalid path. Use /token_status_list/{country}/{doctype}/{id}.",

	// Business logic errors
	ErrNoIndicesAvailable: "No more indices available in the status list.",
	ErrListNotFound:       "Status list not found for the provided URI.",
	ErrIndexOutOfRange:    "Index is out of range for the status list.",

	// Cryptographic errors
	ErrPrivateKeyLoad:     "Failed to load private key.",
	ErrCertificateLoad:    "Failed to load certificate.",
	ErrJWTSigning:         "Failed to sign JWT token.",
	ErrCWTSigning:         "Failed to sign CWT token.",
	ErrCWTEncoding:        "Failed to encode CWT token.",
	ErrUnsupportedKeyType: "Unsupported key type. Only ECDSA keys are supported.",
	ErrEncryptedKey:       "Encrypted private keys are not supported. Please convert to unencrypted PEM format.",

	// System errors
	ErrInternalServer:     "An internal server error occurred.",
	ErrBadRequest:         "Bad request format or parameters.",
	ErrParseForm:          "Failed to parse form data.",
	ErrStatusUpdateFailed: "Failed to update document status.",

	// Storage errors
	ErrStorageBackendInvalid:   "Invalid or unsupported storage backend type.",
	ErrStorageConfigMissing:    "Required storage configuration is missing.",
	ErrStorageConnectionFailed: "Failed to connect to storage backend.",
	ErrStorageOperationFailed:  "Storage operation failed.",
	ErrVersionMismatchCode:     "Version mismatch detected. The file was modified by another process.",
}

// GetErrorMessage returns the message for a given error code
func GetErrorMessage(code ErrorCode) string {
	if message, exists := errorMessages[code]; exists {
		return message
	}

	return errorMessages[ErrInternalServer] // Fallback
}

// WriteError writes a standardized error response
func WriteError(w http.ResponseWriter, statusCode int, errorCode ErrorCode) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Error: ErrorDetail{
			Code:    errorCode,
			Message: GetErrorMessage(errorCode),
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}

// WriteCustomError writes a standardized error response with a custom message
func WriteCustomError(w http.ResponseWriter, statusCode int, errorCode ErrorCode, customMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Error: ErrorDetail{
			Code:    errorCode,
			Message: customMessage,
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}
