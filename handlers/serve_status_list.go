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
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	StatusListJWTContentType = "application/statuslist+jwt"
)

// ServeStatusList serves the status list JWT file for a given country, doctype, and id (rand)
func (h *StatusListHandler) ServeStatusList(w http.ResponseWriter, r *http.Request) {
	// Only allow GET method
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}

	// Parse path: /token_status_list/{country}/{doctype}/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/token_status_list/"), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		WriteError(w, http.StatusBadRequest, ErrInvalidPath)
		return
	}
	country, doctype, rand := parts[0], parts[1], parts[2]

	// Validate country and doctype using existing validation
	if !h.config.ValidateCountry(country) {
		WriteError(w, http.StatusBadRequest, ErrInvalidCountry)
		return
	}
	if !h.config.ValidateDoctype(doctype) {
		WriteError(w, http.StatusBadRequest, ErrInvalidDoctype)
		return
	}

	// Content negotiation - support both JWT and CWT
	accept := r.Header.Get("Accept")
	if accept == "" || accept == "*/*" {
		accept = StatusListJWTContentType // Default to JWT
	}

	var contentType, fileName string
	switch accept {
	case StatusListJWTContentType:
		contentType = StatusListJWTContentType
		fileName = "token_status_list.jwt"
	case "application/statuslist+cwt":
		contentType = "application/statuslist+cwt"
		fileName = "token_status_list.cwt"
	default:
		WriteError(w, http.StatusNotAcceptable, ErrInvalidAccept)
		return
	}

	// Build storage path (platform-independent)
	statusListPath := filepath.Join("token_status_list", country, doctype, rand, fileName)
	// Convert to forward slashes for storage consistency (S3 uses forward slashes)
	statusListPath = filepath.ToSlash(statusListPath)

	// Read file from storage backend
	data, err := h.listManager.GetStorage().Read(statusListPath)
	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			WriteError(w, http.StatusNotFound, ErrListNotFound)
		} else {
			WriteError(w, http.StatusInternalServerError, ErrInternalServer)
		}
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600") // Add security headers for RFC compliance
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write file content
	if _, err := w.Write(data); err != nil {
		// Log error but don't send response as headers are already written
		return
	}
}
