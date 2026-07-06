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
	"context"
	stdcrypto "crypto"
	"testing"
	"time"

	statuslist "github.com/gmb-eudi/go-statuslist"
	"github.com/unknovs/status-list-go/models"
)

// staticFetcher is a network-free statuslist.Fetcher that returns the same
// pre-issued token bytes for any URI (conventions.md: no network in unit
// tests). The gate never needs URI-dependent responses - there is exactly one
// issued list under test.
type staticFetcher []byte

func (f staticFetcher) Get(_ context.Context, _ string) ([]byte, error) {
	return f, nil
}

// TestIssuedTokensVerifyWithGoStatuslist is the conformance regression gate: it
// runs a token this service actually issues (both the ASL-JWT and ASL-CWT
// paths) through the REAL verifier library (github.com/gmb-eudi/go-statuslist),
// not a hand-rolled test decode. It is the permanent net that catches any future
// edit which silently breaks the draft-ietf-oauth-status-list-12 wire format -
// the class of bug (CWT claims at the wrong key, wrong lst type, wrong typ
// header) that Phase B fixed by hand. If GenerateJWT/GenerateCWT ever drift out
// of spec, go-statuslist rejects the token and this test fails in CI.
//
// StatusRef.Format is left as FormatAuto: go-statuslist sniffs JWT (compact JWS)
// vs CWT (CBOR) from the bytes, so no format-switch helper is needed on the
// verify side - the only branch is on which issuer path produced the token.
func TestIssuedTokensVerifyWithGoStatuslist(t *testing.T) {
	for _, format := range []string{"jwt", "cwt"} {
		t.Run(format, func(t *testing.T) {
			key, cfg := newTestKeyCert(t) // reuse Task B1's helper, same package

			statusList := models.NewIssuerStatusList(1, 16, "sequential")
			statusList.StatusList.Set(3, 1) // mark index 3 revoked (bits=1: 1 == INVALID/revoked, Token Status List §7)

			f := NewStatusListFormatter(cfg)
			listURL := "https://issuer.test/statuslist/1"

			// This gate issues a FRESH token each run (iat = now), so keep the expiry
			// dynamic (one year out) — a hardcoded date would eventually make the token
			// verify against its own iat but expire in the verifier, rotting the gate.
			expiryDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

			var raw []byte
			if format == "jwt" {
				s, err := f.GenerateJWT(statusList, "LV", listURL, expiryDate)
				if err != nil {
					t.Fatalf("GenerateJWT: %v", err)
				}
				raw = []byte(s)
			} else {
				b, err := f.GenerateCWT(statusList, "LV", listURL, expiryDate)
				if err != nil {
					t.Fatalf("GenerateCWT: %v", err)
				}
				raw = b
			}

			// Trust our own signer for the gate: the resolver hands the checker the
			// public half of the exact key that issued the token. This isolates the
			// test to wire-format conformance, not trust-anchor resolution (hard rule 6).
			resolve := func(_ context.Context, _ string, _ []byte) (stdcrypto.PublicKey, error) {
				return key.Public(), nil
			}

			checker := statuslist.NewChecker(staticFetcher(raw), nil)

			st, _, err := checker.Check(context.Background(), statuslist.CheckInput{
				Ref: statuslist.StatusRef{
					Kind:   statuslist.RefTokenStatusList,
					URI:    listURL,
					Index:  3,
					Format: statuslist.FormatAuto,
				},
				IssuerKeyResolver: resolve,
				// FailClosed: an issued token that fails to verify must be a hard test
				// failure (non-nil error), never a silently skipped StatusUnknown.
				Policy: statuslist.Policy{FailClosed: true},
			})
			if err != nil {
				t.Fatalf("%s: issued token failed go-statuslist verification: %v", format, err)
			}

			if st != statuslist.StatusRevoked {
				t.Fatalf("%s: index 3 should read revoked, got %v", format, st)
			}
		})
	}
}
