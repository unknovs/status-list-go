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

package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
)

// newTestKeyCert generates an ephemeral P-256 key + self-signed certificate,
// writes them to t.TempDir() in the formats loadPrivateKey/loadCertificate
// accept (PKCS8 "PRIVATE KEY" PEM and DER certificate bytes), and returns the
// key plus a *config.Config pointing at them. Self-contained so it does not rely
// on the CI's openssl-generated temp/ files. Reused by later conformance-gate
// tests.
func newTestKeyCert(t *testing.T) (*ecdsa.PrivateKey, *config.Config) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "status-list-go-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8 key: %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	certPath := filepath.Join(dir, "cert.der")

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	if err := os.WriteFile(certPath, der, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	cfg := &config.Config{
		ServiceURL:  "https://issuer.test/",
		PrivKeyPath: keyPath,
		CertPath:    certPath,
		CountryCode: "LV",
	}

	return key, cfg
}

// keysOf returns the keys of a claims map for use in failure messages.
func keysOf(m map[int64]any) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// protectedTyp unwraps the COSE_Sign1 (CBOR tag 18) and returns COSE protected
// header label 16 (typ, RFC 9596) as a string, or "" if absent.
func protectedTyp(t *testing.T, raw []byte) string {
	t.Helper()

	var tag cbor.RawTag
	if err := cbor.Unmarshal(raw, &tag); err != nil {
		t.Fatalf("decode COSE_Sign1 tag: %v", err)
	}

	if tag.Number != 18 {
		t.Fatalf("expected COSE_Sign1 tag 18, got %d", tag.Number)
	}

	var arr []cbor.RawMessage
	if err := cbor.Unmarshal(tag.Content, &arr); err != nil {
		t.Fatalf("decode COSE_Sign1 array: %v", err)
	}

	if len(arr) != 4 {
		t.Fatalf("COSE_Sign1 must have 4 elements, got %d", len(arr))
	}

	var protectedBytes []byte
	if err := cbor.Unmarshal(arr[0], &protectedBytes); err != nil {
		t.Fatalf("decode protected header bstr: %v", err)
	}

	var protected map[int64]any
	if err := cbor.Unmarshal(protectedBytes, &protected); err != nil {
		t.Fatalf("decode protected header map: %v", err)
	}

	typ, _ := protected[16].(string)

	return typ
}

// coseClaims unwraps the COSE_Sign1 payload and returns the CWT claims map.
// Keys are decoded as int64 (the CWT claim keys are integers; fxamacker converts
// the unsigned CBOR keys to fit the declared int64 map key type).
func coseClaims(t *testing.T, raw []byte) map[int64]any {
	t.Helper()

	var tag cbor.RawTag
	if err := cbor.Unmarshal(raw, &tag); err != nil {
		t.Fatalf("decode COSE_Sign1 tag: %v", err)
	}

	if tag.Number != 18 {
		t.Fatalf("expected COSE_Sign1 tag 18, got %d", tag.Number)
	}

	var arr []cbor.RawMessage
	if err := cbor.Unmarshal(tag.Content, &arr); err != nil {
		t.Fatalf("decode COSE_Sign1 array: %v", err)
	}

	if len(arr) != 4 {
		t.Fatalf("COSE_Sign1 must have 4 elements, got %d", len(arr))
	}

	var payloadBytes []byte
	if err := cbor.Unmarshal(arr[2], &payloadBytes); err != nil {
		t.Fatalf("decode payload bstr: %v", err)
	}

	var claims map[int64]any
	if err := cbor.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("decode CWT claims: %v", err)
	}

	return claims
}

// stringKeysOf returns the keys of a string-keyed claims map for use in
// failure messages.
func stringKeysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// TestGenerateJWTHasTTLClaim asserts GenerateJWT (the live, production-consumed
// ASL-JWT issuer path) adds an additive top-level ttl claim so a verifier can
// cache the token, while everything else about the JWT's wire shape - sub/iat/
// exp, status_list.bits, status_list.lst's base64url TEXT encoding, and the
// typ header - stays exactly as before (the no-break invariant for this task).
func TestGenerateJWTHasTTLClaim(t *testing.T) {
	_, cfg := newTestKeyCert(t)

	f := NewStatusListFormatter(cfg)

	statusList := models.NewIssuerStatusList(1, 16, "sequential")

	tokenString, err := f.GenerateJWT(statusList, "LV", "https://issuer.test/statuslist/1", "2030-01-01")
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("expected compact JWS (header.payload.signature), got %d segments", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header segment: %v", err)
	}

	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}

	if typ, _ := header["typ"].(string); typ != "statuslist+jwt" {
		t.Fatalf("typ header must still be %q, got %q", "statuslist+jwt", typ)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload segment: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	ttl, ok := payload["ttl"].(float64)
	if !ok {
		t.Fatalf("ttl claim must be present, got keys %v", stringKeysOf(payload))
	}

	if ttl != 3600 {
		t.Fatalf("ttl mismatch: got %v, want 3600 (matching GenerateCWT's ttl convention)", ttl)
	}

	for _, key := range []string{"sub", "iat", "exp"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("%s claim must still be present, got keys %v", key, stringKeysOf(payload))
		}
	}

	sl, ok := payload["status_list"].(map[string]interface{})
	if !ok {
		t.Fatalf("status_list must still be present as an object, got %T", payload["status_list"])
	}

	lst, ok := sl["lst"].(string)
	if !ok {
		t.Fatalf("status_list.lst must still be a base64url TEXT string (unlike the CWT path), got %T", sl["lst"])
	}

	if _, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(lst); err != nil {
		t.Fatalf("status_list.lst must still decode as unpadded base64url text: %v", err)
	}
}

// TestGenerateCWTIsDraft12 asserts the CWT emitted by GenerateCWT conforms to
// draft-ietf-oauth-status-list-12: status_list at claim 65533 with a raw CBOR
// byte-string lst, ttl at claim 65534, and COSE protected header typ (label 16).
func TestGenerateCWTIsDraft12(t *testing.T) {
	_, cfg := newTestKeyCert(t)

	f := NewStatusListFormatter(cfg)

	statusList := models.NewIssuerStatusList(1, 16, "sequential")

	raw, err := f.GenerateCWT(statusList, "LV", "https://issuer.test/statuslist/1", "2030-01-01")
	if err != nil {
		t.Fatalf("GenerateCWT: %v", err)
	}

	claims := coseClaims(t, raw)

	sl, ok := claims[int64(65533)].(map[any]any)
	if !ok {
		t.Fatalf("status_list must be claim 65533, got keys %v", keysOf(claims))
	}

	if _, isBytes := sl["lst"].([]byte); !isBytes {
		t.Fatalf("lst must be a CBOR byte string, got %T", sl["lst"])
	}

	if _, ok := claims[int64(65534)]; !ok {
		t.Fatalf("ttl (65534) must be present, got keys %v", keysOf(claims))
	}

	if typ := protectedTyp(t, raw); typ != "application/statuslist+cwt" {
		t.Fatalf("typ(16) mismatch: %q", typ)
	}
}
