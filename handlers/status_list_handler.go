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

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/debuglog"
	"github.com/unknovs/status-list-go/errors"
	"github.com/unknovs/status-list-go/services"
	"github.com/unknovs/status-list-go/services/storage"
)

const (
	APIKeyHeader      = "X-Api-Key"
	ContentTypeHeader = "Content-Type"
)

// StatusListHandler handles status list related requests
type StatusListHandler struct {
	config      *config.Config
	listManager *services.ListManager
}

// NewStatusListHandler creates a new status list handler
func NewStatusListHandler(cfg *config.Config, stor storage.Storage) *StatusListHandler {
	return &StatusListHandler{
		config:      cfg,
		listManager: services.NewListManager(cfg, stor),
	}
}

// TakeIndex handles the take index request
// @Summary Take Index
// @Description Takes a new index from the status list
// @Tags token_status_list
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param X-API-Key header string true "API Key"
// @Param doctype formData string true "Document type"
// @Param country formData string true "Country code"
// @Param expiry_date formData string true "Expiry date (YYYY-MM-DD)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /token_status_list/take [post]
func (h *StatusListHandler) TakeIndex(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	debuglog.Printf("TakeIndex: request received from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		errors.WriteError(w, http.StatusMethodNotAllowed, errors.ErrBadRequest)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrParseForm)
		return
	}

	// Validate API key
	apiKey := r.Header.Get(APIKeyHeader)
	if apiKey != h.config.APIKey {
		log.Printf("Authentication failed: incorrect API key provided")
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrInvalidAPIKey)
		return
	}

	// Validate doctype
	doctype := r.FormValue("doctype")
	if !h.config.ValidateDoctype(doctype) {
		log.Printf("Invalid document type provided: %q", doctype)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	// Validate country
	country := r.FormValue("country")
	if !h.config.ValidateCountry(country) {
		log.Printf("Invalid country provided: %q", country)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}

	// Validate expiry date
	expiryDate := r.FormValue("expiry_date")
	if err := h.validateExpiryDate(expiryDate); err != nil {
		log.Printf("Invalid expiry date provided, error: %v", err)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidExpiryDate)
		return
	}

	debuglog.Printf("TakeIndex: doctype=%s country=%s expiry=%s", doctype, country, expiryDate)

	// Generate status list info
	statusInfo, err := h.listManager.GenerateStatusListInfo(country, doctype, expiryDate)
	if err != nil {
		debuglog.Printf("TakeIndex: GenerateStatusListInfo failed after %s: %v", time.Since(start), err)
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrInternalServer)
		return
	}

	debuglog.Printf("TakeIndex: completed in %s ā€” Status Info: %+v", time.Since(start), statusInfo)
	h.writeJSON(w, http.StatusOK, statusInfo)
}

// GetIndex handles the get index request
// @Summary Get Token Status
// @Description Retrieves the status of a token from the revocation list
// @Tags token_status_list
// @Accept json
// @Produce plain
// @Param uri query string true "URI of the status list"
// @Param id query string false "Identifier of the token"
// @Param idx query string false "Index of the status list"
// @Success 200 {string} string "Status value"
// @Failure 400 {object} map[string]string
// @Router /token_status_list/get [get]
func (h *StatusListHandler) GetIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errors.WriteError(w, http.StatusMethodNotAllowed, errors.ErrBadRequest)
		return
	}

	log.Printf("Get request received")

	uri := r.URL.Query().Get("uri")
	id := r.URL.Query().Get("id")
	idx := r.URL.Query().Get("idx")

	// Use id if idx is not provided
	if idx == "" {
		idx = id
	}

	if uri == "" || idx == "" {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrBadRequest)
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidIndex)
		return
	}

	// Decode URI
	decodedURI, err := url.QueryUnescape(uri)
	if err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	// Load list and get status
	status, err := h.listManager.GetStatusFromURI(decodedURI, index)
	if err != nil {
		log.Printf("Failed to get status: %v", err)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrListNotFound)
		return
	}

	w.Header().Set(ContentTypeHeader, "text/plain")
	fmt.Fprintf(w, "%d", status)
}

// SetIndex handles the set index request
// @Summary Set Token Status
// @Description Sets or updates the status of a token in the revocation list
// @Tags token_status_list
// @Accept application/x-www-form-urlencoded
// @Produce plain
// @Param X-API-Key header string true "API Key"
// @Param uri formData string true "URI of the status list"
// @Param id formData string false "Identifier of the token"
// @Param idx formData string false "Index of the status list"
// @Param status formData string true "New status value"
// @Success 200 {string} string "Status Changed"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /token_status_list/set [post]
func (h *StatusListHandler) SetIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errors.WriteError(w, http.StatusMethodNotAllowed, errors.ErrBadRequest)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrParseForm)
		return
	}

	// Validate API key
	apiKey := r.Header.Get(APIKeyHeader)
	if apiKey != h.config.APIKey {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrUnauthorizedAccess)
		return
	}

	uri := r.FormValue("uri")
	id := r.FormValue("id")
	idx := r.FormValue("idx")
	statusStr := r.FormValue("status")

	// Use id if idx is not provided
	if idx == "" {
		idx = id
	}

	if uri == "" || idx == "" || statusStr == "" {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrBadRequest)
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		log.Printf("Invalid index: %v", err)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidIndex)
		return
	}

	status, err := strconv.Atoi(statusStr)
	if err != nil {
		log.Printf("Invalid status: %v", err)
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidStatus)
		return
	}

	if status != 1 {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidStatus)
		return
	}

	// Parse URI to extract country, doctype, and id
	parsedURL, err := url.Parse(uri)
	if err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	normalizedPath, ok := normalizeStatusListPath(parsedURL.Path, h.config.BasePath)
	if !ok {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	pathParts := strings.Split(normalizedPath, "/")
	if len(pathParts) != 3 {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	country := pathParts[0]
	doctype := pathParts[1]
	listID := pathParts[2]

	// Validate extracted values
	if !h.config.ValidateCountry(country) {
		log.Printf("Invalid country from URI provided")
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}

	if !h.config.ValidateDoctype(doctype) {
		log.Printf("Invalid doctype from URI provided")
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	// Update status
	err = h.listManager.SetStatus(uri, country, doctype, listID, index, status)
	if err != nil {
		log.Printf("Failed to set status: %v", err)
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrStatusUpdateFailed)
		return
	}

	w.Header().Set(ContentTypeHeader, "text/plain")
	fmt.Fprintf(w, "Status Changed\n")
}

// Helper methods for JSON responses
func (h *StatusListHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set(ContentTypeHeader, "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// validateExpiryDate validates the expiry date format and ensures it's in the future
func (h *StatusListHandler) validateExpiryDate(expiryDate string) error {
	parsedDate, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return fmt.Errorf("invalid expiry date format. Use YYYY-MM-DD")
	}

	if parsedDate.Before(time.Now().Truncate(24 * time.Hour)) {
		return fmt.Errorf("expiry date must be in the future")
	}

	return nil
}

