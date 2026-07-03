# status-list-go — Agent Work Plan

## Codebase context

- **Module:** `github.com/unknovs/status-list-go`
- **Root:** `MAINLOGIC/status-list-go/`
- **Framework:** `azugo.io/azugo` (but handlers currently bypass it — see todo `rewrite-handlers-azugo`)
- **Platform kit:** `github.com/gmb-lib/go-platform-kit` — used by all other services in the monorepo; not yet wired into this service
- **Go version:** 1.26.4

### Key packages

| Path | Purpose |
|---|---|
| `app/app.go` | App bootstrap — `NewApp`, `Run`, CORS setup |
| `config/config.go` | Config loaded from raw `os.Getenv` |
| `handlers/status_list_handler.go` | `TakeIndex`, `GetIndex`, `SetIndex` — currently `net/http` handlers |
| `handlers/serve_status_list.go` | `ServeStatusList` — serves JWT/CWT files |
| `routes/router.go` | Route wiring — wraps handlers via `fasthttpadaptor` |
| `services/list_manager.go` | Allocates indices, dumps lists to storage |
| `services/status_list_format.go` | Generates JWT and CWT tokens |
| `models/status_list.go` | `StatusList`, `Allocator`, `IssuerStatusList`, `StatusListData` |
| `services/storage/` | `Storage` interface, local + S3 backends |
| `renewal/renewal.go` | Background worker — re-signs JWT/CWT daily |
| `cleanup/cleanup.go` | Background worker — deletes expired lists daily |
| `errors/errors.go`, `errors/http.go` | Custom error codes + JSON writer (to be deleted) |
| `debuglog/debuglog.go` | Debug log toggle — `LOG_LEVEL=debug` (to be deleted) |

### Reference services (correct patterns to follow)

- `MAINLOGIC/issuer-go/`
- `MAINLOGIC/idauth/`
- `MAINLOGIC/api-wallet-digimaks/`
- `MAINLOGIC/go-platform-kit/` — shared library

### go-platform-kit packages used by other services

| Import | What it does |
|---|---|
| `github.com/gmb-lib/go-platform-kit/platform` | `platform.Setup(app, opts)` — one call wires OTel, redaction, correlation, error handler |
| `github.com/gmb-lib/go-platform-kit/errors` | `pkerrors.HTTP(domain, reason)`, `pkerrors.InternalError{}`, `pkerrors.NewProblem(code)` |
| `github.com/gmb-lib/go-platform-kit/config` | `config.BaseConfiguration` to embed in service config |
| `github.com/gmb-lib/go-platform-kit/correlation` | `correlation.FromContext(ctx)` for trace/correlation IDs |
| `github.com/gmb-lib/go-platform-kit/observability` | `observability.EnableTracing(app, cfg)` |

---

## Coding conventions (must follow)

- No comments unless the WHY is non-obvious
- No error handling for impossible cases; trust framework guarantees
- Handlers use `*azugo.Context` — never `http.ResponseWriter` / `*http.Request`
- Route handlers live in `handlers/`, request/response types in the same package
- Set `Cache-Control: no-store` on endpoints returning sensitive credential material
- Format: `gofumpt`. Lint: `golangci-lint`. No `--no-verify` bypasses
- No per-file license headers (root `LICENSE` is sufficient)

---

## Todo list

16 tasks total. Check off with `UPDATE todos SET status = 'done' WHERE id = 'X'` when complete.

### Phase 2 — Bug fixes (start here, no dependencies)

#### `fix-cwt-hex` — Fix CWT binary encoding 🔴
**Files:** `services/status_list_format.go`, `services/list_manager.go`

`GenerateCWT` and `GenerateIdentifierCWT` return `hex.EncodeToString(cwtData)`. CWT is binary CBOR — storing or serving it as a hex string is wrong. Consumers requesting `application/statuslist+cwt` receive garbage.

**Fix:**
1. Change return types of `GenerateCWT` / `GenerateIdentifierCWT` from `(string, error)` to `([]byte, error)`.
2. Remove `hex.EncodeToString` — return `cwtData` directly.
3. In `list_manager.go` → `saveTokenStatusListFormats` / `saveIdentifierListFormats`: the `.cwt` path currently does `[]byte(cwtContent)` — this now receives `[]byte` directly, so just pass it.
4. In `handlers/serve_status_list.go` → `ServeStatusList`: `data` is already `[]byte` from storage, written directly — no change needed there.

---

#### `fix-jwt-claims` — Fix JWTClaims field duplication 🔴
**File:** `services/status_list_format.go`

`JWTClaims` manually declares `Subject string \`json:"sub"\`` and `IssuedAt int64 \`json:"iat"\`` while also embedding `jwt.RegisteredClaims` (which already has these fields). The manual fields shadow the embedded ones and cause JSON serialization ambiguity.

**Fix:**
1. In `JWTClaims`: remove the manual `Subject` and `IssuedAt` fields. Use `RegisteredClaims.Subject` (string) and `RegisteredClaims.IssuedAt` (`*jwt.NumericDate`) instead.
2. Same fix for `IdentifierJWTClaims`: remove manual `Issuer`, `Subject`, `IssuedAt` fields; use the embedded `RegisteredClaims` equivalents.
3. Update the claim construction sites: `jwt.NewNumericDate(time.Now())` for `IssuedAt`.

---

#### `add-exp-claim` — Add `exp` claim to JWT and CWT tokens 🔴
**Files:** `services/status_list_format.go`, `services/list_manager.go`

Neither `GenerateJWT` nor `GenerateCWT` sets an `exp` claim. The `StatusListData.Expires` field exists but is never used in token generation. Per IETF Token Status List, `exp` is required.

**Fix:**
1. Add `expiryDate string` parameter to `GenerateJWT`, `GenerateCWT`, `GenerateIdentifierJWT`, `GenerateIdentifierCWT`.
2. Parse `expiryDate` (`"2006-01-02"`) and set:
   - JWT: `RegisteredClaims.ExpiresAt = jwt.NewNumericDate(parsedDate)`
   - CWT: add `4: parsedDate.Unix()` (key `4` = `exp` per RFC 8392)
3. In `list_manager.go`: `saveTokenStatusListFormats` and `saveIdentifierListFormats` already have `statusListData` — pass `*statusListData.Expires` through to the formatter calls.

---

#### `fix-s3-stat-check` — Fix S3-incompatible `os.Stat` in renewal
**File:** `renewal/renewal.go`

`RenewLists()` starts with:
```go
if _, err := os.Stat(rs.config.StatusListDir); os.IsNotExist(err) { ... }
```
This only works for local storage. For S3 it either silently passes or incorrectly errors.

**Fix:** Delete the `os.Stat` block entirely. `rs.storage.List("")` (called a few lines later) already returns an error if the backend is inaccessible — that is the correct availability check.

---

### Phase 1 — Framework alignment

#### `rewrite-handlers-azugo` — Rewrite handlers to native `azugo.Context` 🟠
**Files:** `handlers/status_list_handler.go`, `handlers/serve_status_list.go`

All handlers currently implement `func(http.ResponseWriter, *http.Request)` and are bridged via `fasthttpadaptor` in the router. They must be converted to `func(ctx *azugo.Context)`.

**API mapping:**
| net/http | azugo.Context |
|---|---|
| `r.Method` | remove — routing guarantees method |
| `r.ParseForm()` / `r.FormValue(k)` | `ctx.Form().Get(k)` |
| `r.URL.Query().Get(k)` | `ctx.QueryParam(k)` |
| `r.Header.Get(k)` | `string(ctx.Header.Peek(k))` |
| `w.Header().Set(k,v)` | `ctx.Header.Set(k,v)` |
| `w.WriteHeader(code)` | `ctx.StatusCode(code)` |
| `json.NewEncoder(w).Encode(v)` | `ctx.JSON(v)` |
| `fmt.Fprintf(w, "%d", n)` | `ctx.SetBodyString(strconv.Itoa(n))` |
| `w.Write(data)` | `ctx.SetBodyRaw(data)` |
| `errors.WriteError(w, code, errCode)` | `ctx.Error(...)` _(after platform-setup; or keep interim helper until then)_ |

Path params in `ServeStatusList` (`{country}`, `{doctype}`, `{id}`):
```go
country := ctx.Params().GetString("country")
doctype := ctx.Params().GetString("doctype")
rand    := ctx.Params().GetString("id")
```

The `normalizeStatusListPath` helper becomes unnecessary once path params are used directly — delete it.

The `StatusListHandler` methods change signature from:
```go
func (h *StatusListHandler) TakeIndex(w http.ResponseWriter, r *http.Request)
```
to:
```go
func (h *StatusListHandler) TakeIndex(ctx *azugo.Context)
```

---

#### `simplify-router` — Simplify `routes/router.go` (depends on: `rewrite-handlers-azugo`)
**File:** `routes/router.go`

Once handlers are native Azugo, the entire adapter machinery is obsolete.

**Remove:**
- `fasthttpadaptor` import
- `adapt()` helper function
- `createRegisterFunc` and its closure
- `fasthttpadaptor.NewFastHTTPHandler` usage everywhere

**Replace with direct registration:**
```go
app.Post(prefix("/token_status_list/take"), handler.TakeIndex)
app.Post(prefix("/token_status_list/set"), handler.SetIndex)
app.Get(prefix("/token_status_list/get"), handler.GetIndex)
app.Get(prefix("/token_status_list/{country}/{doctype}/{id}"), handler.ServeStatusList)
```

Keep `createPrefixFunc` / base-path logic as-is (or inline it if trivial after simplification).

---

#### `fix-double-cors` — Remove duplicate CORS in `app.go`
**File:** `app/app.go`

Two CORS setups exist:
1. `azApp.RouterOptions().CORS.SetOrigins("*")` + `SetHeaders(...)`
2. Custom `corsMiddleware` doing the same headers manually + handling OPTIONS

**Fix:** Delete `corsMiddleware` func and `azApp.Use(corsMiddleware)`. Keep only the `RouterOptions().CORS` setup — Azugo handles OPTIONS preflight automatically.

---

#### `fix-newapp-errors` — `NewApp` returns errors instead of `log.Fatalf`
**Files:** `app/app.go`, `main.go`

**Fix:**
1. Change `func NewApp(cfg *config.Config) *App` → `func NewApp(cfg *config.Config) (*App, error)`.
2. Replace each `log.Fatalf(...)` with `return nil, fmt.Errorf(...)`.
3. In `main.go` → `runServer()`: `application, err := app.NewApp(cfg)` + `if err != nil { return err }`.

---

### Phase 4 — go-platform-kit integration

#### `add-platform-kit-dep` — Add `go-platform-kit` to `go.mod`
**File:** `go.mod`

Check the version used in `api-wallet-digimaks/go.mod` or `issuer-go/go.mod` and add the same:
```
require github.com/gmb-lib/go-platform-kit vX.Y.Z
```
Then `go mod tidy`.

---

#### `migrate-config-viper` — Migrate config to Azugo viper + `BaseConfiguration` (depends on: `add-platform-kit-dep`)
**File:** `config/config.go`

The current config uses raw `os.Getenv`. Must migrate to Azugo's viper-based system and embed `*pkgconfig.BaseConfiguration` to provide `ServiceName` and `Telemetry` for `platform.Setup`.

**Pattern (from go-platform-kit/config/config.go):**
```go
type Configuration struct {
    *pkgconfig.BaseConfiguration `mapstructure:",squash"`

    APIKey              string `mapstructure:"api_key"`
    ServiceURL          string `mapstructure:"service_url"`
    // ... all other fields
}

func NewConfiguration() *Configuration {
    return &Configuration{BaseConfiguration: pkgconfig.New()}
}

func (c *Configuration) Bind(_ string, v *viper.Viper) {
    c.BaseConfiguration.Bind("", v)
    _ = v.BindEnv("api_key", "API_KEY")
    _ = v.BindEnv("service_url", "SERVICE_URL")
    // ... bind all env vars
    v.SetDefault("api_key", "test")
    v.SetDefault("service_url", "http://localhost:8080/")
    // ... other defaults
}

func (c *Configuration) Validate(valid *validation.Validate) error {
    return valid.Struct(c)
}
```

The `Load()` function is replaced by Azugo's config loading mechanism via `server.New`. The `ensureDir` calls and `normalizeHour`/`normalizeMinute` helpers can move to a `PostLoad()` or be kept in `Load()` as a post-Bind step.

---

#### `platform-setup` — Call `platform.Setup` in `app.go` (depends on: `migrate-config-viper`, `add-platform-kit-dep`)
**File:** `app/app.go`

After `server.New(...)` and before route registration:
```go
if err := platform.Setup(azApp, platform.Options{
    Config: cfg.BaseConfiguration,
}); err != nil {
    return nil, fmt.Errorf("platform setup: %w", err)
}
```

This single call installs (in order):
1. **OTel tracing** — middleware traces every handler; inert when no `OTEL_EXPORTER_OTLP_ENDPOINT`
2. **Log redaction** — wraps the app logger; no secret/PII escapes
3. **Correlation middleware** — binds `correlation_id`, `trace_id`, `span_id` to each request and all log lines
4. **RFC 9457 error handler** — every `ctx.Error(err)` renders as `application/problem+json` with `trace_id`

---

#### `replace-error-responses` — Replace `errors.WriteError` with `ctx.Error` (depends on: `platform-setup`, `rewrite-handlers-azugo`)
**Files:** `handlers/status_list_handler.go`, `handlers/serve_status_list.go`

After `platform.Setup` installs the global error handler, replace manual error JSON with:

```go
// 401
ctx.Error(pkerrors.HTTP("request", "unauthorized"))

// 400 bad request (with detail)
ctx.Error(pkerrors.HTTP("statusList", "invalid", "expiry date must be in the future"))

// 404
ctx.Error(pkerrors.HTTP("statusList", "notFound"))

// 406 Not Acceptable — needs RegisterReason at startup:
// pkerrors.RegisterReason("notAcceptable", pkerrors.ReasonSpec{Status: 406, Title: "Not acceptable"})
ctx.Error(pkerrors.HTTP("statusList", "notAcceptable"))

// 500
ctx.Error(pkerrors.InternalError{Err: err})
```

After all `errors.WriteError` / `errors.WriteCustomError` calls are replaced, **delete** the entire `errors/` package.

---

#### `structured-logging-workers` — Structured zap logging in workers (depends on: `platform-setup`)
**Files:** `renewal/renewal.go`, `cleanup/cleanup.go`, `debuglog/debuglog.go`, `app/app.go`

1. Add `logger *zap.Logger` field to `RenewalService` and `cleanup.Service`.
2. Pass `azApp.Log()` from `app.go` into `StartRenewalThread` and `StartCleanupWorker`.
3. Replace all `log.Printf(...)` with `rs.logger.Info(...)` / `rs.logger.Error(...)` / `rs.logger.Debug(...)`.
4. Delete `debuglog/debuglog.go` — `logger.Debug(...)` handles it natively; set `LOG_LEVEL=debug` maps to zap's debug level via Azugo.
5. For otel spans in workers:
```go
ctx, span := otel.Tracer("status-list").Start(context.Background(), "renewal")
defer span.End()
```

---

### Phase 3 — Code quality

#### `api-key-middleware` — Extract API key to middleware (depends on: `rewrite-handlers-azugo`)
**Files:** `routes/router.go` or `app/app.go`, `handlers/status_list_handler.go`

Create an Azugo middleware:
```go
func apiKeyMiddleware(apiKey string) azugo.RequestHandlerFunc {
    return func(next azugo.RequestHandler) azugo.RequestHandler {
        return func(ctx *azugo.Context) {
            if string(ctx.Header.Peek("X-Api-Key")) != apiKey {
                ctx.Error(pkerrors.HTTP("request", "unauthorized"))
                return
            }
            next(ctx)
        }
    }
}
```
Apply it only to the internal-only routes (`/take`, `/set`) via a route group or inline `app.Use` scoped to those routes.
Remove the duplicated API key checks from `TakeIndex` and `SetIndex`.

---

#### `remove-license-headers` — Remove per-file Apache license headers
**All `.go` files**

Strip the `/* Copyright (c) Gatis Beikerts ... */` block from every `.go` file. The root `LICENSE` file is sufficient. Other services in the monorepo carry no per-file headers.

Quick approach:
```sh
# Review first, then apply
grep -rl "Licensed under the Apache License" . --include="*.go"
```

---

#### `remove-health-flag` — Remove `--health-check` CLI flag
**File:** `main.go`

Both `--health-check` flag on the root command and the `health` subcommand call `performHealthCheck()`.

**Fix:** Remove `healthCheckFlag` variable, `cmd.Flags().BoolVar(...)`, and the `if healthCheckFlag` branch from `RunE`. Keep only the `health` subcommand.

---

## Dependency graph

```
fix-cwt-hex          (ready)
fix-jwt-claims       (ready)
add-exp-claim        (ready)
fix-s3-stat-check    (ready)
fix-double-cors      (ready)
fix-newapp-errors    (ready)
remove-license-headers (ready)
remove-health-flag   (ready)

rewrite-handlers-azugo (ready)
  └── simplify-router
  └── api-key-middleware
  └── replace-error-responses (also needs platform-setup)

add-platform-kit-dep (ready)
  └── migrate-config-viper
        └── platform-setup
              └── replace-error-responses
              └── structured-logging-workers
```

## Execution order

Do phases in this order to avoid rework:

1. `fix-cwt-hex`, `fix-jwt-claims`, `add-exp-claim` — isolated bug fixes, merge first
2. `fix-s3-stat-check`, `fix-double-cors`, `fix-newapp-errors`, `remove-license-headers`, `remove-health-flag` — small independent cleanups
3. `rewrite-handlers-azugo` → `simplify-router` — framework alignment
4. `add-platform-kit-dep` → `migrate-config-viper` → `platform-setup` — platform integration
5. `replace-error-responses` → `structured-logging-workers` — depends on both 3 and 4
6. `api-key-middleware` — depends on 3

## Verification

After each task, run:
```sh
cd status-list-go
go build ./...
go test ./...
golangci-lint run
```
