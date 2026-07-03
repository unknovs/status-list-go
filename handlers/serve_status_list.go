package handlers

import (
	stdErrors "errors"
	"fmt"
	"path/filepath"
	"strings"

	"azugo.io/azugo"

	"github.com/unknovs/status-list-go/errors"
	"github.com/valyala/fasthttp"
)

const (
	StatusListJWTContentType = "application/statuslist+jwt"
	StatusListCWTContentType = "application/statuslist+cwt"
)

// ServeStatusList serves the status list JWT/CWT file for the given country, doctype, and id.
func (h *StatusListHandler) ServeStatusList(ctx *azugo.Context) {
	country := ctx.Params.String("country")
	doctype := ctx.Params.String("doctype")
	rand := ctx.Params.String("id")

	if !h.config.ValidateCountry(country) {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidCountry)
		return
	}
	if !h.config.ValidateDoctype(doctype) {
		writeError(ctx, fasthttp.StatusBadRequest, errors.ErrInvalidDoctype)
		return
	}

	accept := ctx.Header.Get("Accept")
	if accept == "" || accept == "*/*" {
		accept = StatusListJWTContentType
	}

	var contentType, fileName string
	switch parseAcceptHeader(accept) {
	case StatusListJWTContentType:
		contentType = StatusListJWTContentType
		fileName = "token_status_list.jwt"
	case StatusListCWTContentType:
		contentType = StatusListCWTContentType
		fileName = "token_status_list.cwt"
	default:
		writeError(ctx, fasthttp.StatusNotAcceptable, errors.ErrInvalidAccept)
		return
	}

	statusListPath := filepath.ToSlash(filepath.Join("token_status_list", country, doctype, rand, fileName))

	data, err := h.listManager.GetStorage().Read(statusListPath)
	if err != nil {
		if stdErrors.Is(err, errors.ErrNotFound) {
			writeError(ctx, fasthttp.StatusNotFound, errors.ErrListNotFound)
		} else {
			writeError(ctx, fasthttp.StatusInternalServerError, errors.ErrInternalServer)
		}
		return
	}

	ctx.Header.Set("Content-Type", contentType)
	ctx.Header.Set("Cache-Control", "no-store")
	ctx.Header.Set("X-Content-Type-Options", "nosniff")
	ctx.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	ctx.Raw(data)
}

// parseAcceptHeader extracts the first supported media type from the Accept header.
func parseAcceptHeader(accept string) string {
	for _, mediaType := range strings.Split(accept, ",") {
		mt := strings.TrimSpace(mediaType)
		if idx := strings.Index(mt, ";"); idx != -1 {
			mt = mt[:idx]
		}
		mt = strings.TrimSpace(mt)

		switch mt {
		case StatusListJWTContentType:
			return StatusListJWTContentType
		case StatusListCWTContentType:
			return StatusListCWTContentType
		case "*/*", "application/*":
			return StatusListJWTContentType
		}
	}
	return ""
}
