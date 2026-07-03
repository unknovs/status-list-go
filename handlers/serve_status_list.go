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
	"path/filepath"
	"strings"

	"azugo.io/azugo"
	pkerrors "github.com/gmb-lib/go-platform-kit/errors"

	localerrors "github.com/unknovs/status-list-go/errors"
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
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid country code"))
		return
	}
	if !h.config.ValidateDoctype(doctype) {
		ctx.Error(pkerrors.HTTP("statusList", "invalid", "invalid document type"))
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
		ctx.Error(pkerrors.HTTP("statusList", "notAcceptable"))
		return
	}

	statusListPath := filepath.ToSlash(filepath.Join("token_status_list", country, doctype, rand, fileName))

	data, err := h.listManager.GetStorage().Read(statusListPath)
	if err != nil {
		if stdErrors.Is(err, localerrors.ErrNotFound) {
			ctx.Error(pkerrors.HTTP("statusList", "notFound"))
		} else {
			ctx.Error(pkerrors.InternalError{Err: err})
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
