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
	stdErrors "errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/unknovs/status-list-go/errors"
)

const (
	StatusListJWTContentType = "application/statuslist+jwt"
	StatusListCWTContentType = "application/statuslist+cwt"
)

// ServeStatusList serves the status list JWT file for a given country, doctype, and id (rand)
func (h *StatusListHandler) ServeStatusList(w http.ResponseWriter, r *http.Request) {
	// Only allow GET method
	if r.Method != http.MethodGet {
		errors.WriteError(w, http.StatusMethodNotAllowed, errors.ErrBadRequest)
		return
	}

	// Normalize path by stripping configured base path when present.
	path, ok := normalizeStatusListPath(r.URL.Path, h.config.BasePath)
	if !ok {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidPath)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidPath)
		return
	}
	country, doctype, rand := parts[0], parts[1], parts[2]

	// Validate country and doctype using existing validation
	if !h.config.ValidateCountry(country) {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}
	if !h.config.ValidateDoctype(doctype) {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	// Content negotiation - support both JWT and CWT
	// Parse Accept header to handle multiple media types (e.g., "application/statuslist+jwt,application/json")
	accept := r.Header.Get("Accept")
	if accept == "" || accept == "*/*" {
		accept = StatusListJWTContentType // Default to JWT
	}

	var contentType, fileName string
	acceptedType := parseAcceptHeader(accept)

	switch acceptedType {
	case StatusListJWTContentType:
		contentType = StatusListJWTContentType
		fileName = "token_status_list.jwt"
	case StatusListCWTContentType:
		contentType = StatusListCWTContentType
		fileName = "token_status_list.cwt"
	default:
		errors.WriteError(w, http.StatusNotAcceptable, errors.ErrInvalidAccept)
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
		if stdErrors.Is(err, errors.ErrNotFound) {
			errors.WriteError(w, http.StatusNotFound, errors.ErrListNotFound)
		} else {
			errors.WriteError(w, http.StatusInternalServerError, errors.ErrInternalServer)
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

// parseAcceptHeader extracts the first supported media type from Accept header
// Handles comma-separated values like "application/statuslist+jwt,application/json,text/html"
func parseAcceptHeader(accept string) string {
	// Split by comma to handle multiple media types
	mediaTypes := strings.Split(accept, ",")

	for _, mediaType := range mediaTypes {
		// Trim whitespace and remove quality values (e.g., ";q=0.9")
		mt := strings.TrimSpace(mediaType)
		if idx := strings.Index(mt, ";"); idx != -1 {
			mt = mt[:idx]
		}
		mt = strings.TrimSpace(mt)

		// Check if this is a supported media type (prefer JWT over CWT)
		if mt == StatusListJWTContentType {
			return StatusListJWTContentType
		}
		if mt == StatusListCWTContentType {
			return StatusListCWTContentType
		}
		// Wildcards
		if mt == "*/*" || mt == "application/*" {
			return StatusListJWTContentType // Default to JWT
		}
	}

	// No supported media type found
	return ""
}

func normalizeStatusListPath(rawPath, basePath string) (string, bool) {
	path := rawPath
	if basePath != "" && strings.HasPrefix(path, basePath) {
		path = strings.TrimPrefix(path, basePath)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}

	if !strings.HasPrefix(path, "/token_status_list/") {
		return "", false
	}

	trimmed := strings.TrimPrefix(path, "/token_status_list/")
	return trimmed, true
}
