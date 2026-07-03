package handlers

import (
	"bytes"
	"testing"

	"github.com/unknovs/status-list-go/errors"
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
		expectedError       errors.ErrorCode
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
			expectedError:  errors.ErrInvalidAccept,
		},
		{
			name:           "invalid country",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/EE/PID/test123",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusBadRequest,
			expectedError:  errors.ErrInvalidCountry,
		},
		{
			name:           "invalid doctype",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/INVALID/test123",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusBadRequest,
			expectedError:  errors.ErrInvalidDoctype,
		},
		{
			name:           "file not found",
			method:         fasthttp.MethodGet,
			path:           "/token_status_list/LV/PID/missing",
			headers:        map[string]string{"Accept": StatusListJWTContentType},
			expectedStatus: fasthttp.StatusNotFound,
			expectedError:  errors.ErrListNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeRequest(t, ta.app, tt.method, tt.path, "", tt.headers)
			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, got)
			}

			if tt.expectedError != "" {
				payload := decodeErrorResponse(t, resp)
				if payload.Error.Code != tt.expectedError {
					t.Fatalf("expected error code %s, got %s", tt.expectedError, payload.Error.Code)
				}
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

func TestServeStatusListRouteValidation(t *testing.T) {
	ta := newTestApp(t)
	createStatusListFile(t, ta, "LV", "PID", "test123", "token_status_list.jwt", []byte("jwt-content"))

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{name: "post not allowed", method: fasthttp.MethodPost, path: "/token_status_list/LV/PID/test123", expectedStatus: fasthttp.StatusMethodNotAllowed},
		{name: "too few segments", method: fasthttp.MethodGet, path: "/token_status_list/LV/PID", expectedStatus: fasthttp.StatusNotFound},
		{name: "too many segments", method: fasthttp.MethodGet, path: "/token_status_list/LV/PID/test123/extra", expectedStatus: fasthttp.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeRequest(t, ta.app, tt.method, tt.path, "", map[string]string{"Accept": StatusListJWTContentType})
			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, got)
			}
		})
	}
}

func TestServeStatusListResponseHeaders(t *testing.T) {
	ta := newTestApp(t)
	createStatusListFile(t, ta, "LV", "PID", "test123", "token_status_list.jwt", []byte("jwt-content"))

	resp := executeRequest(t, ta.app, fasthttp.MethodGet, "/token_status_list/LV/PID/test123", "", map[string]string{"Accept": StatusListJWTContentType})
	if got := resp.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status 200, got %d", got)
	}

	headers := map[string]string{
		"Content-Type":           StatusListJWTContentType,
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
		"Content-Length":         "11",
	}
	for name, want := range headers {
		if got := string(resp.Header.Peek(name)); got != want {
			t.Fatalf("expected %s=%q, got %q", name, want, got)
		}
	}
}

func TestParseAcceptHeader(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   string
	}{
		{name: "jwt", accept: StatusListJWTContentType, want: StatusListJWTContentType},
		{name: "cwt", accept: StatusListCWTContentType, want: StatusListCWTContentType},
		{name: "application wildcard", accept: "application/*", want: StatusListJWTContentType},
		{name: "weighted list", accept: "text/plain, application/statuslist+cwt;q=0.8", want: StatusListCWTContentType},
		{name: "unsupported", accept: "text/plain", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAcceptHeader(tt.accept); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
