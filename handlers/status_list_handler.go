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
	"net/url"
	"strconv"
	"strings"
	"time"

	"azugo.io/azugo"
	pkerrors "github.com/gmb-lib/go-platform-kit/errors"

	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/services"
	"github.com/unknovs/status-list-go/services/storage"
)

const APIKeyHeader = "X-Api-Key"

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

func formStr(ctx *azugo.Context, key string) string {
	if v := ctx.Form.StringOptional(key); v != nil {
		return *v
	}
	return ""
}

// TakeIndex handles the take index request
func (h *StatusListHandler) TakeIndex(ctx *azugo.Context) {
	doctype := formStr(ctx, "doctype")
	if !h.config.ValidateDoctype(doctype) {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid document type"))
		return
	}

	country := formStr(ctx, "country")
	if !h.config.ValidateCountry(country) {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid country code"))
		return
	}

	expiryDate := formStr(ctx, "expiry_date")
	if err := validateExpiryDate(expiryDate); err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", err.Error()))
		return
	}

	statusInfo, err := h.listManager.GenerateStatusListInfo(country, doctype, expiryDate)
	if err != nil {
		ctx.Error(pkerrors.InternalError{Err: err})
		return
	}

	ctx.JSON(statusInfo)
}

// GetIndex handles the get index request
func (h *StatusListHandler) GetIndex(ctx *azugo.Context) {
	var uri, idx string
	if v := ctx.Query.StringOptional("uri"); v != nil {
		uri = *v
	}
	if v := ctx.Query.StringOptional("idx"); v != nil {
		idx = *v
	}
	if idx == "" {
		if v := ctx.Query.StringOptional("id"); v != nil {
			idx = *v
		}
	}

	if uri == "" || idx == "" {
		ctx.Error(pkerrors.HTTP("request", "invalid", "uri and idx are required"))
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid index value"))
		return
	}

	decodedURI, err := url.QueryUnescape(uri)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid URI encoding"))
		return
	}

	status, err := h.listManager.GetStatusFromURI(decodedURI, index)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "notFound"))
		return
	}

	ctx.Header.Set("Content-Type", "text/plain")
	ctx.Text(strconv.Itoa(status))
}

// SetIndex handles the set index request
func (h *StatusListHandler) SetIndex(ctx *azugo.Context) {
	uri := formStr(ctx, "uri")
	idx := formStr(ctx, "idx")
	if idx == "" {
		idx = formStr(ctx, "id")
	}
	statusStr := formStr(ctx, "status")

	if uri == "" || idx == "" || statusStr == "" {
		ctx.Error(pkerrors.HTTP("request", "invalid", "uri, idx/id, and status are required"))
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid index value"))
		return
	}

	status, err := strconv.Atoi(statusStr)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid status value"))
		return
	}

	if status != 1 {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "status must be 1"))
		return
	}

	parsedURL, err := url.Parse(uri)
	if err != nil {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid URI format"))
		return
	}

	normalizedPath, ok := normalizeURIPath(parsedURL.Path, h.config.BasePath)
	if !ok {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "URI does not match token_status_list path"))
		return
	}

	pathParts := strings.Split(normalizedPath, "/")
	if len(pathParts) != 3 {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid URI structure"))
		return
	}

	country := pathParts[0]
	doctype := pathParts[1]
	listID := pathParts[2]

	if !h.config.ValidateCountry(country) {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid country in URI"))
		return
	}

	if !h.config.ValidateDoctype(doctype) {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid doctype in URI"))
		return
	}

	if err := h.listManager.SetStatus(uri, country, doctype, listID, index, status); err != nil {
		ctx.Error(pkerrors.InternalError{Err: err})
		return
	}

	ctx.Header.Set("Content-Type", "text/plain")
	ctx.Text(fmt.Sprintf("Status Changed\n"))
}

func validateExpiryDate(expiryDate string) error {
	parsedDate, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return fmt.Errorf("invalid expiry date format, expected YYYY-MM-DD")
	}

	if parsedDate.Before(time.Now().Truncate(24 * time.Hour)) {
		return fmt.Errorf("expiry date must be in the future")
	}

	return nil
}

// normalizeURIPath strips the list-type prefix and base path from a status list URI path,
// returning the "{country}/{doctype}/{id}" segment.
func normalizeURIPath(rawPath, basePath string) (string, bool) {
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

	return strings.TrimPrefix(path, "/token_status_list/"), true
}
