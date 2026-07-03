package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"azugo.io/azugo"
	azugoconfig "azugo.io/azugo/config"
	"azugo.io/core"
	pkerrors "github.com/gmb-lib/go-platform-kit/errors"
	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
	"github.com/unknovs/status-list-go/services/storage"
	"github.com/valyala/fasthttp"
)

type testApp struct {
	app     *azugo.App
	config  *config.Config
	storage storage.Storage
	handler *StatusListHandler
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	rootDir, err := os.MkdirTemp(".", ".status-list-handler-test-")
	if err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(rootDir) })

	statusDir := filepath.Join(rootDir, "status")
	backupDir := filepath.Join(rootDir, "backup")
	logDir := filepath.Join(rootDir, "logs")
	for _, dir := range []string{statusDir, backupDir, logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}

	cfg := &config.Config{
		APIKey:              "test-api-key",
		ServiceURL:          "http://localhost:8080/",
		TokenStatusListSize: 100,
		StatusListDir:       statusDir,
		BackupDir:           backupDir,
		LogDir:              logDir,
		PrivKeyPath:         filepath.Join(rootDir, "missing-key.pem"),
		CertPath:            filepath.Join(rootDir, "missing-cert.der"),
		CountryCode:         "LV",
		BackendType:         "local",
		AllowedDoctypes:     map[string]bool{"PID": true, "MDL": true},
	}

	stor, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	handler := NewStatusListHandler(cfg, stor)
	a := azugo.New()
	a.AppName = "test"
	a.AppVer = "1.0"
	appCfg := azugoconfig.New()
	a.SetConfig(nil, appCfg)
	a.App.SetConfig(nil, appCfg.Core())
	if err := appCfg.Load(nil, appCfg, string(core.EnvironmentDevelopment)); err != nil {
		t.Fatalf("failed to load azugo config: %v", err)
	}
	a.Post("/token_status_list/take", handler.TakeIndex)
	a.Post("/token_status_list/set", handler.SetIndex)
	a.Get("/token_status_list/get", handler.GetIndex)
	a.Get("/token_status_list/{country}/{doctype}/{id}", handler.ServeStatusList)

	pkerrors.RegisterReason("notAcceptable", pkerrors.ReasonSpec{Status: 406, Title: "Not acceptable"})

	return &testApp{
		app:     a,
		config:  cfg,
		storage: stor,
		handler: handler,
	}
}

func executeRequest(t *testing.T, app *azugo.App, method, path, body string, headers map[string]string) *fasthttp.Response {
	t.Helper()

	var req fasthttp.Request
	req.Header.SetMethod(method)
	req.SetRequestURI(path)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != "" {
		req.SetBodyString(body)
	}

	var ctx fasthttp.RequestCtx
	ctx.Init(&req, nil, nil)
	app.Handler(&ctx)

	var resp fasthttp.Response
	ctx.Response.CopyTo(&resp)
	return &resp
}

func executeFormRequest(t *testing.T, app *azugo.App, path string, form url.Values, headers map[string]string) *fasthttp.Response {
	t.Helper()

	allHeaders := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	for k, v := range headers {
		allHeaders[k] = v
	}

	return executeRequest(t, app, fasthttp.MethodPost, path, form.Encode(), allHeaders)
}

func checkErrorStatus(t *testing.T, resp *fasthttp.Response, expectedStatus int) {
	t.Helper()
	if got := resp.StatusCode(); got != expectedStatus {
		t.Fatalf("expected status %d, got %d (body: %s)", expectedStatus, got, string(resp.Body()))
	}
}

func createStoredStatusList(t *testing.T, ta *testApp, country, doctype, randID string, activeIndexes ...int) string {
	t.Helper()

	statusList := models.NewIssuerStatusList(1, 100, "random")
	identifierList := make(map[string]int)
	for _, idx := range activeIndexes {
		statusList.StatusList.Set(idx, 1)
		identifierList[strconv.Itoa(idx)] = 1
	}

	expires := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	uri := fmt.Sprintf("http://localhost:8080/token_status_list/%s/%s/%s", country, doctype, randID)
	identifierURI := fmt.Sprintf("http://localhost:8080/identifier_list/%s/%s/%s", country, doctype, randID)
	statusListData := &models.StatusListData{
		TokenStatusList:   statusList,
		IdentifierList:    identifierList,
		Expires:           &expires,
		Rand:              randID,
		Country:           country,
		Doctype:           doctype,
		StatusListURI:     uri,
		IdentifierListURI: identifierURI,
	}

	jsonData, err := json.Marshal(statusListData)
	if err != nil {
		t.Fatalf("failed to marshal status list data: %v", err)
	}

	path := filepath.ToSlash(filepath.Join("token_status_list", country, doctype, randID, "full_list.json"))
	if err := ta.storage.Create(path, jsonData); err != nil {
		t.Fatalf("failed to seed status list data: %v", err)
	}

	return uri
}

func TestNewStatusListHandler(t *testing.T) {
	ta := newTestApp(t)

	if ta.handler == nil {
		t.Fatal("expected handler to be created")
	}
	if ta.handler.config != ta.config {
		t.Fatal("expected handler config to match test config")
	}
	if ta.handler.listManager == nil {
		t.Fatal("expected list manager to be initialized")
	}
}

func TestTakeIndex(t *testing.T) {
	ta := newTestApp(t)
	futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	tests := []struct {
		name           string
		headers        map[string]string
		form           url.Values
		expectedStatus int
		assertSuccess  bool
	}{
		{
			name:           "invalid doctype",
			form:           url.Values{"doctype": {"INVALID"}, "country": {"LV"}, "expiry_date": {futureDate}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid country",
			form:           url.Values{"doctype": {"PID"}, "country": {"EE"}, "expiry_date": {futureDate}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid expiry format",
			form:           url.Values{"doctype": {"PID"}, "country": {"LV"}, "expiry_date": {"2026/01/01"}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "missing expiry date",
			form:           url.Values{"doctype": {"PID"}, "country": {"LV"}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "valid request",
			headers:        map[string]string{APIKeyHeader: ta.config.APIKey},
			form:           url.Values{"doctype": {"PID"}, "country": {"LV"}, "expiry_date": {futureDate}},
			expectedStatus: fasthttp.StatusOK,
			assertSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeFormRequest(t, ta.app, "/token_status_list/take", tt.form, tt.headers)

			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, got)
			}

			if !tt.assertSuccess {
				return
			}

			if got := string(resp.Header.Peek("Content-Type")); got != "application/json" {
				t.Fatalf("expected content type application/json, got %s", got)
			}

			var payload models.StatusListInfo
			if err := json.Unmarshal(resp.Body(), &payload); err != nil {
				t.Fatalf("failed to decode success payload: %v", err)
			}
			if !strings.Contains(payload.StatusList.URI, "/token_status_list/LV/PID/") {
				t.Fatalf("unexpected status list uri: %s", payload.StatusList.URI)
			}
			if !strings.Contains(payload.IdentifierList.URI, "/identifier_list/LV/PID/") {
				t.Fatalf("unexpected identifier list uri: %s", payload.IdentifierList.URI)
			}
			if payload.IdentifierList.ID != strconv.Itoa(payload.StatusList.Idx) {
				t.Fatalf("expected identifier id %d, got %s", payload.StatusList.Idx, payload.IdentifierList.ID)
			}
		})
	}
}

func TestGetIndex(t *testing.T) {
	ta := newTestApp(t)
	uri := createStoredStatusList(t, ta, "LV", "PID", "test-rand", 7)


	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid request with idx",
			path:           "/token_status_list/get?" + url.Values{"uri": {uri}, "idx": {"7"}}.Encode(),
			expectedStatus: fasthttp.StatusOK,
			expectedBody:   "1",
		},
		{
			name:           "valid request with id alias",
			path:           "/token_status_list/get?" + url.Values{"uri": {uri}, "id": {"0"}}.Encode(),
			expectedStatus: fasthttp.StatusOK,
			expectedBody:   "0",
		},
		{
			name:           "missing uri",
			path:           "/token_status_list/get?" + url.Values{"idx": {"0"}}.Encode(),
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid index",
			path:           "/token_status_list/get?" + url.Values{"uri": {uri}, "idx": {"invalid"}}.Encode(),
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "non-existent list",
			path:           "/token_status_list/get?" + url.Values{"uri": {"http://localhost:8080/token_status_list/LV/PID/missing"}, "idx": {"0"}}.Encode(),
			expectedStatus: fasthttp.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := executeRequest(t, ta.app, fasthttp.MethodGet, tt.path, "", nil)

			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, got)
			}

			if tt.expectedBody != "" {
				if got := string(resp.Body()); got != tt.expectedBody {
					t.Fatalf("expected body %s, got %s", tt.expectedBody, got)
				}
				if got := string(resp.Header.Peek("Content-Type")); !strings.HasPrefix(got, "text/plain") {
					t.Fatalf("expected content type starting with text/plain, got %s", got)
				}
			}
		})
	}
}

func TestSetIndex(t *testing.T) {
	ta := newTestApp(t)

	tests := []struct {
		name             string
		headers          map[string]string
		form             url.Values
		setupRandID      string
		expectedStatus   int
		expectedBodyPart string
		verifyPath       string
		expectedGetBody  string
	}{
		{
			name:             "valid request with idx",
			form:             url.Values{"idx": {"0"}, "status": {"1"}},
			setupRandID:      "set-idx",
			expectedStatus:   fasthttp.StatusOK,
			expectedBodyPart: "Status Changed",
			expectedGetBody:  "1",
		},
		{
			name:             "valid request with id alias",
			form:             url.Values{"id": {"1"}, "status": {"1"}},
			setupRandID:      "set-id",
			expectedStatus:   fasthttp.StatusOK,
			expectedBodyPart: "Status Changed",
			expectedGetBody:  "1",
		},
		{
			name:           "invalid status",
			form:           url.Values{"uri": {"http://localhost:8080/token_status_list/LV/PID/test"}, "idx": {"0"}, "status": {"2"}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid uri path",
			form:           url.Values{"uri": {"http://localhost:8080/short"}, "idx": {"0"}, "status": {"1"}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
		{
			name:           "invalid country in uri",
			form:           url.Values{"uri": {"http://localhost:8080/token_status_list/EE/PID/test"}, "idx": {"0"}, "status": {"1"}},
			expectedStatus: fasthttp.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			for k, values := range tt.form {
				copied := append([]string(nil), values...)
				form[k] = copied
			}

			if tt.setupRandID != "" {
				uri := createStoredStatusList(t, ta, "LV", "PID", tt.setupRandID)
				form.Set("uri", uri)
				tt.verifyPath = "/token_status_list/get?" + url.Values{"uri": {uri}, "idx": {form.Get("idx")}}.Encode()
				if form.Get("idx") == "" {
					tt.verifyPath = "/token_status_list/get?" + url.Values{"uri": {uri}, "idx": {form.Get("id")}}.Encode()
				}
			}

			resp := executeFormRequest(t, ta.app, "/token_status_list/set", form, tt.headers)
			if got := resp.StatusCode(); got != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.expectedStatus, got, string(resp.Body()))
			}

			if tt.expectedBodyPart == "" {
				return
			}

			if got := string(resp.Header.Peek("Content-Type")); !strings.HasPrefix(got, "text/plain") {
				t.Fatalf("expected content type starting with text/plain, got %s", got)
			}
			if body := strings.TrimSpace(string(resp.Body())); !strings.Contains(body, tt.expectedBodyPart) {
				t.Fatalf("expected body to contain %q, got %q", tt.expectedBodyPart, body)
			}

			verifyResp := executeRequest(t, ta.app, fasthttp.MethodGet, tt.verifyPath, "", nil)
			if got := verifyResp.StatusCode(); got != fasthttp.StatusOK {
				t.Fatalf("expected follow-up get status 200, got %d", got)
			}
			if got := string(verifyResp.Body()); got != tt.expectedGetBody {
				t.Fatalf("expected updated status %s, got %s", tt.expectedGetBody, got)
			}
		})
	}
}

func TestValidateExpiryDate(t *testing.T) {
	tests := []struct {
		name        string
		expiryDate  string
		expectError bool
	}{
		{name: "future date", expiryDate: time.Now().AddDate(0, 0, 30).Format("2006-01-02")},
		{name: "today", expiryDate: time.Now().Format("2006-01-02")},
		{name: "invalid format", expiryDate: "2026/12/31", expectError: true},
		{name: "past date", expiryDate: "2020-01-01", expectError: true},
		{name: "empty", expiryDate: "", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExpiryDate(tt.expiryDate)
			if tt.expectError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
