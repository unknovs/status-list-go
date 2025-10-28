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
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/unknovs/status-list-go/config"
	"github.com/unknovs/status-list-go/models"
	"github.com/veraison/go-cose"
)

var (
	ErrServiceURLEmpty = errors.New("ServiceURL is empty")
)

const (
	ErrCountryNotSupportedFormat = "country not supported: %s (expected: %s)"
	ErrLoadPrivateKeyFormat      = "failed to load private key: %v"
	ErrLoadCertificateFormat     = "failed to load certificate: %v"
	ErrEncodeCWTFormat           = "failed to encode CWT: %v"
	ErrSignCWTFormat             = "failed to sign CWT: %v"
)

// CWTClaims represents the CBOR Web Token claims for status list
type CWTClaims struct {
	Issuer     string                 `cbor:"1,keyasint,omitempty"` // iss
	Subject    string                 `cbor:"2,keyasint,omitempty"` // sub
	Audience   string                 `cbor:"3,keyasint,omitempty"` // aud
	Expiration int64                  `cbor:"4,keyasint,omitempty"` // exp
	NotBefore  int64                  `cbor:"5,keyasint,omitempty"` // nbf
	IssuedAt   int64                  `cbor:"6,keyasint,omitempty"` // iat
	CWTID      []byte                 `cbor:"7,keyasint,omitempty"` // cti
	StatusList map[string]interface{} `cbor:"65534,keyasint"`       // status_list claim
}
type JWTClaims struct {
	Subject    string                 `json:"sub"`
	IssuedAt   int64                  `json:"iat"`
	StatusList map[string]interface{} `json:"status_list"`
	jwt.RegisteredClaims
}

// IdentifierJWTClaims represents the JWT claims for identifier list
type IdentifierJWTClaims struct {
	Issuer         string         `json:"iss"`
	Subject        string         `json:"sub"`
	IssuedAt       int64          `json:"iat"`
	IdentifierList map[string]int `json:"identifier_list"`
	jwt.RegisteredClaims
}

// StatusListFormatter handles JWT and CWT formatting
type StatusListFormatter struct {
	config *config.Config
}

// NewStatusListFormatter creates a new formatter
func NewStatusListFormatter(cfg *config.Config) *StatusListFormatter {
	return &StatusListFormatter{config: cfg}
}

// GenerateJWT creates a JWT for the token status list
func (f *StatusListFormatter) GenerateJWT(tokenStatusList *models.IssuerStatusList, country, listURL string) (string, error) {
	// Validate country matches configuration
	if !f.config.ValidateCountry(country) {
		return "", fmt.Errorf(ErrCountryNotSupportedFormat, country, f.config.CountryCode)
	}

	// Get certificate paths
	privKeyPath, certPath := f.config.GetCertificatePaths()

	// Load private key
	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	// Load certificate
	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	// Create JWT claims
	claims := JWTClaims{
		Subject:  listURL,
		IssuedAt: time.Now().Unix(),
		StatusList: map[string]interface{}{
			"bits": 1,
			"lst":  base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(tokenStatusList.StatusList.Compressed()),
		},
	}

	// Create token with custom headers
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["typ"] = "statuslist+jwt"
	token.Header["x5c"] = []string{base64.StdEncoding.EncodeToString(cert.Raw)}

	// Sign the token
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %v", err)
	}

	return tokenString, nil
}

// GenerateCWT creates a CWT for the token status list
func (f *StatusListFormatter) GenerateCWT(tokenStatusList *models.IssuerStatusList, country, listURL string) (string, error) {
	// Get certificate paths
	privKeyPath, certPath := f.config.GetCertificatePaths()

	// Load private key
	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	// Load certificate
	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	// Create CWT claims using numeric keys as per RFC 8392
	now := time.Now()

	// Safely handle ServiceURL
	issuer := f.config.ServiceURL
	if issuer == "" {
		return "", ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	// Use raw map with integer keys for proper CWT format
	claims := map[interface{}]interface{}{
		1: issuer,     // iss (issuer) - RFC 8392 claim 1
		2: listURL,    // sub (subject) - RFC 8392 claim 2
		6: now.Unix(), // iat (issued at) - RFC 8392 claim 6
		65534: map[string]interface{}{ // status_list claim - draft-ietf-oauth-status-list-02
			"bits": 1,
			"lst":  base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(tokenStatusList.StatusList.Compressed()),
		},
	}

	// Encode claims to CBOR
	claimsData, err := cbor.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	// Create COSE Sign1 message
	// Set up headers with proper structure
	headers := cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm:   cose.AlgorithmES256, // ECDSA using P-256 and SHA-256 (-7)
			cose.HeaderLabelContentType: "application/statuslist+cwt",
		},
		Unprotected: cose.UnprotectedHeader{
			// Include certificate in x5c header (label 33)
			33: []interface{}{cert.Raw},
		},
	}

	// Create COSE signer
	signer, err := cose.NewSigner(cose.AlgorithmES256, privateKey)
	if err != nil {
		return "", fmt.Errorf(ErrSignCWTFormat, err)
	}

	// Create and sign the COSE Sign1 message
	msg := &cose.Sign1Message{
		Headers: headers,
		Payload: claimsData,
	}

	// Use crypto/rand.Reader for randomness source and empty external AAD
	err = msg.Sign(rand.Reader, []byte{}, signer)
	if err != nil {
		return "", fmt.Errorf(ErrSignCWTFormat, err)
	}

	// Encode to CBOR
	cwtData, err := cbor.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	// Return hex encoded CWT to match the expected format
	return hex.EncodeToString(cwtData), nil
}

// GenerateIdentifierJWT creates a JWT for the identifier list
func (f *StatusListFormatter) GenerateIdentifierJWT(identifierList map[string]int, country, listURL string) (string, error) {
	// Validate country matches configuration
	if !f.config.ValidateCountry(country) {
		return "", fmt.Errorf(ErrCountryNotSupportedFormat, country, f.config.CountryCode)
	}

	// Get certificate paths
	privKeyPath, certPath := f.config.GetCertificatePaths()

	// Load private key
	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	// Load certificate
	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	// Create JWT claims
	issuer := f.config.ServiceURL
	if issuer == "" {
		return "", ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	claims := IdentifierJWTClaims{
		Issuer:         issuer, // Safely handle ServiceURL
		Subject:        listURL,
		IssuedAt:       time.Now().Unix(),
		IdentifierList: identifierList,
	}

	// Create token with custom headers
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["typ"] = "statuslist+jwt" // Use RFC compliant base type
	token.Header["x5c"] = []string{base64.StdEncoding.EncodeToString(cert.Raw)}

	// Sign the token
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %v", err)
	}

	return tokenString, nil
}

// GenerateIdentifierCWT creates a CWT for the identifier list
func (f *StatusListFormatter) GenerateIdentifierCWT(identifierList map[string]int, country, listURL string) (string, error) {
	// Validate country matches configuration
	if !f.config.ValidateCountry(country) {
		return "", fmt.Errorf(ErrCountryNotSupportedFormat, country, f.config.CountryCode)
	}

	// Get certificate paths
	privKeyPath, certPath := f.config.GetCertificatePaths()

	// Load private key
	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	// Load certificate
	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	// Create CWT claims for identifier list
	// RFC-compliant structure using status list claim format
	now := time.Now()

	// Safely handle ServiceURL
	issuer := f.config.ServiceURL
	if issuer == "" {
		return "", ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	claims := map[interface{}]interface{}{
		1: issuer,     // iss (issuer) - RFC 8392 claim 1
		2: listURL,    // sub (subject) - RFC 8392 claim 2
		6: now.Unix(), // iat (issued at) - RFC 8392 claim 6
		65534: map[string]interface{}{ // status_list claim - RFC compliant claim number
			"bits":           1,              // Identifier lists use 1 bit per entry
			"identifier_map": identifierList, // Custom field for identifier mapping
		},
	}

	// Encode claims to CBOR
	claimsData, err := cbor.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	// Create COSE Sign1 message
	// Set up headers
	headers := cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm: cose.AlgorithmES256, // ECDSA using P-256 and SHA-256
			// Use a more specific content type for identifier lists
			cose.HeaderLabelContentType: "application/statuslist+cwt", // RFC compliant base type
		},
		Unprotected: cose.UnprotectedHeader{
			// Include certificate in x5c header (label 33)
			33: []interface{}{cert.Raw},
		},
	}

	// Create COSE signer
	signer, err := cose.NewSigner(cose.AlgorithmES256, privateKey)
	if err != nil {
		return "", fmt.Errorf(ErrSignCWTFormat, err)
	}

	// Create and sign the COSE Sign1 message
	msg := &cose.Sign1Message{
		Headers: headers,
		Payload: claimsData,
	}

	// Use crypto/rand.Reader for randomness source and empty external AAD
	err = msg.Sign(rand.Reader, []byte{}, signer)
	if err != nil {
		return "", fmt.Errorf(ErrSignCWTFormat, err)
	}

	// Encode to CBOR
	cwtData, err := cbor.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	// Return hex encoded CWT to match the expected format
	return hex.EncodeToString(cwtData), nil
}

// loadPrivateKey loads a private key from file
func (f *StatusListFormatter) loadPrivateKey(keyPath string) (*ecdsa.PrivateKey, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Find the PEM block, ignoring any bag attributes or other content
	var block *pem.Block
	remaining := keyData
	for {
		block, remaining = pem.Decode(remaining)
		if block == nil {
			return nil, fmt.Errorf("failed to find any PEM block in key file")
		}
		// Look for private key blocks
		if strings.Contains(block.Type, "PRIVATE KEY") {
			break
		}
		if len(remaining) == 0 {
			return nil, fmt.Errorf("no private key block found in PEM file")
		}
	}

	var privateKey interface{}
	var keyBytes []byte = block.Bytes

	// Only support unencrypted PEM and modern PKCS#8 encrypted keys
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		// This is a PKCS#8 encrypted private key - requires external decryption
		return nil, fmt.Errorf("PKCS#8 encrypted private keys require external decryption. Please decrypt the key first: openssl pkcs8 -in encrypted_key.pem -out decrypted_key.pem")
	}

	// For unencrypted keys, try different formats
	// 1. Try PKCS8 format first (most common format)
	privateKey, err = x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		// 2. Try EC private key format
		privateKey, err = x509.ParseECPrivateKey(keyBytes)
		if err != nil {
			// 3. Try PKCS1 RSA key format (in case it's RSA)
			_, rsaErr := x509.ParsePKCS1PrivateKey(keyBytes)
			if rsaErr == nil {
				return nil, fmt.Errorf("RSA keys are not supported, need ECDSA key")
			}
			// Return the original EC parsing error
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
	}

	ecdsaKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA private key, got type: %T", privateKey)
	}

	return ecdsaKey, nil
}

// loadCertificate loads a certificate from file
func (f *StatusListFormatter) loadCertificate(certPath string) (*x509.Certificate, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	// Try DER format first
	cert, err := x509.ParseCertificate(certData)
	if err == nil {
		return cert, nil
	}

	// Try PEM format
	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate")
	}

	return x509.ParseCertificate(block.Bytes)
}
