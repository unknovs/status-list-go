package handlers

import (
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"azugo.io/azugo"
	"github.com/valyala/fasthttp"

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

func writeError(ctx *azugo.Context, statusCode int, errorCode errors.ErrorCode) {
	ctx.StatusCode(statusCode)
	ctx.Header.Set(ContentTypeHeader, "application/json")
	ctx.JSON(errors.ErrorResponse{
		Error: errors.ErrorDetail{
			Code:    errorCode,
			Message: errors.GetErrorMessage(errorCode),
		},
	})
}

func formStr(ctx *azugo.Context, key string) string {
	if v := ctx.Form.StringOptional(key); v != nil {
		return *v
	}
	return ""
}

// TakeIndex handles the take index request
func (h *StatusListHandler) TakeIndex(ctx *azugo.Context) {
	start := time.Now()
	debuglog.Printf("TakeIndex: request received")

	apiKey := ctx.Header.Get(APIKeyHeader)
	if apiKey != h.config.APIKey {
		log.Printf("Authentication failed: incorrect API key provided")
		writeError(ctx, fasthttp.StatusUnauthorized, errors.ErrInvalidAPIKey)
		return
	}

	doctype := formStr(ctx, "doctype")
	if !h.config.ValidateDoctype(doctype) {
		log.Printf("Invalid document type provided: %q", doctype)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	country := formStr(ctx, "country")
	if !h.config.ValidateCountry(country) {
		log.Printf("Invalid country provided: %q", country)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}

	expiryDate := formStr(ctx, "expiry_date")
	if err := h.validateExpiryDate(expiryDate); err != nil {
		log.Printf("Invalid expiry date provided, error: %v", err)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidExpiryDate)
		return
	}

	debuglog.Printf("TakeIndex: doctype=%s country=%s expiry=%s", doctype, country, expiryDate)

	statusInfo, err := h.listManager.GenerateStatusListInfo(country, doctype, expiryDate)
	if err != nil {
		debuglog.Printf("TakeIndex: GenerateStatusListInfo failed after %s: %v", time.Since(start), err)
		writeError(ctx, fasthttp.StatusInternalServerError, errors.ErrInternalServer)
		return
	}

	debuglog.Printf("TakeIndex: completed in %s", time.Since(start))
	ctx.JSON(statusInfo)
}

// GetIndex handles the get index request
func (h *StatusListHandler) GetIndex(ctx *azugo.Context) {
	log.Printf("Get request received")

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
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrBadRequest)
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidIndex)
		return
	}

	decodedURI, err := url.QueryUnescape(uri)
	if err != nil {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	status, err := h.listManager.GetStatusFromURI(decodedURI, index)
	if err != nil {
		log.Printf("Failed to get status: %v", err)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrListNotFound)
		return
	}

	ctx.Header.Set(ContentTypeHeader, "text/plain")
	ctx.Text(strconv.Itoa(status))
}

// SetIndex handles the set index request
func (h *StatusListHandler) SetIndex(ctx *azugo.Context) {
	apiKey := ctx.Header.Get(APIKeyHeader)
	if apiKey != h.config.APIKey {
		writeError(ctx, fasthttp.StatusUnauthorized, errors.ErrUnauthorizedAccess)
		return
	}

	uri := formStr(ctx, "uri")
	idx := formStr(ctx, "idx")
	if idx == "" {
		idx = formStr(ctx, "id")
	}
	statusStr := formStr(ctx, "status")

	if uri == "" || idx == "" || statusStr == "" {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrBadRequest)
		return
	}

	index, err := strconv.Atoi(idx)
	if err != nil {
		log.Printf("Invalid index: %v", err)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidIndex)
		return
	}

	status, err := strconv.Atoi(statusStr)
	if err != nil {
		log.Printf("Invalid status: %v", err)
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidStatus)
		return
	}

	if status != 1 {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidStatus)
		return
	}

	parsedURL, err := url.Parse(uri)
	if err != nil {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	normalizedPath, ok := normalizeURIPath(parsedURL.Path, h.config.BasePath)
	if !ok {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	pathParts := strings.Split(normalizedPath, "/")
	if len(pathParts) != 3 {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidURI)
		return
	}

	country := pathParts[0]
	doctype := pathParts[1]
	listID := pathParts[2]

	if !h.config.ValidateCountry(country) {
		log.Printf("Invalid country from URI provided")
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}

	if !h.config.ValidateDoctype(doctype) {
		log.Printf("Invalid doctype from URI provided")
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	if err := h.listManager.SetStatus(uri, country, doctype, listID, index, status); err != nil {
		log.Printf("Failed to set status: %v", err)
		writeError(ctx, fasthttp.StatusInternalServerError, errors.ErrStatusUpdateFailed)
		return
	}

	ctx.Header.Set(ContentTypeHeader, "text/plain")
	ctx.Text(fmt.Sprintf("Status Changed\n"))
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
