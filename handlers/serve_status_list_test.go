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
	"bytes"
	"testing"

	"github.com/valyala/fasthttp"
)

func createStatusListFile(t *testing.T, ta *testApp, country, doctype, randID, fileName string, content []byte) {
	t.Helper()

	path := "token_status_list/" + country + "/" + doctype + "/" + randID + "/" + fileName
	if err := ta.storage.Create(path, content); err != nil {
		t.Fatalf("failed to create test status list file: %v", err)
	}
}

func TestServeStatusList(t *testing.T) {
	ta := newTestApp(t)
	jwtContent := []byte("jwt-content")
	cwtContent := []byte{0x01, 0x02, 0x03}
	createStatusListFile(t, ta, "LV", "PID", "test123", "token_status_list.jwt", jwtContent)
	createStatusListFile(t, ta, "LV", "PID", "test123", "token_status_list.cwt", cwtContent)

	tests := []struct {
		name                string
		method              string
		path                string
		headers             map[string]string
		expectedStatus      int
		expectedContentType string
		expectedBody        []byte
	}{
		{
			name:                "jwt request",
			method:              fasthttp.MethodGet,
			path:                "/token_status_list/LV/PID/test123",
			headers:             map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus:      fasthttp.StatusOK,
			expectedContentType: StatusListJWTContentType,
			expectedBody:        jwtContent,
		},
		{
			name:                "cwt request",
			method:              fasthttp.MethodGet,
			path:                "/token_status_list/LV/PID/test123",
			headers:             map[string]string{"Accept": StatusListCWTContentType},
			expectedStatus:      fasthttp.StatusOK,
			expectedContentType: StatusListCWTContentType,
			expectedBody:        cwtContent,
		},
		{
			name:                "default accept",
			method:              fasthttp.MethodGet,
			path:                "/token_status_list/LV/PID/test123",
			expectedStatus:      fasthttp.StatusOK,
			expectedContentType: StatusListJWTContentType,
			expectedBody:        jwtContent,
		},
		{
			name:                "wildcard accept",
			method:              fasthttp.MethodGet,
			path:                "/token_status_list/LV/PID/test123",
			headers:             map[string]string{"Accept": "*/*"},
			expectedStatus:      fasthttp.StatusOK,
			expectedContentType: StatusListJWTContentType,
			expectedBody:        jwtContent,
		},
		{
			name:           "invalid accept",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/PID/test123",
			headers:        map[string]string{"Accept": "application/json"},
			expectedStatus: fasthttp.StatusNotAcceptable,
		},
		{
			name:           "invalid country",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/EE/PID/test123",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid doctype",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/INVALID/test123",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "file not found",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/PID/missing",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeRequest(t, ta.app, tt.method, tt.path, "", tt.headers)
			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.expectedStatus, got, string(resp.Body()))
			}

			if tt.expectedBody == nil {
				return
			}

			if got := string(resp.Header.Peek("Content-Type")); got != tt.expectedContentType {
				t.Fatalf("expected content type %s, got %s", tt.expectedContentType, got)
			}
			if !bytes.Equal(resp.Body(), tt.expectedBody) {
				t.Fatalf("expected body %v, got %v", tt.expectedBody, resp.Body())
			}
		})
	}
}
