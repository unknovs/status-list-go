package services

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
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
	StatusList map[string]interface{} `json:"status_list"`
	jwt.RegisteredClaims
}

// IdentifierJWTClaims represents the JWT claims for identifier list
type IdentifierJWTClaims struct {
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
func (f *StatusListFormatter) GenerateJWT(tokenStatusList *models.IssuerStatusList, country, listURL, expiryDate string) (string, error) {
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
	expiry, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return "", fmt.Errorf("invalid expiry date: %w", err)
	}
	claims := JWTClaims{
		StatusList: map[string]interface{}{
			"bits": 1,
			"lst":  base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(tokenStatusList.StatusList.Compressed()),
		},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   listURL,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiry),
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
func (f *StatusListFormatter) GenerateCWT(tokenStatusList *models.IssuerStatusList, country, listURL, expiryDate string) ([]byte, error) {
	// Get certificate paths
	privKeyPath, certPath := f.config.GetCertificatePaths()

	// Load private key
	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return nil, fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	// Load certificate
	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return nil, fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	now := time.Now()

	issuer := f.config.ServiceURL
	if issuer == "" {
		return nil, ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	expiry, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry date: %w", err)
	}

	claims := map[interface{}]interface{}{
		1: issuer,          // iss
		2: listURL,         // sub
		4: expiry.Unix(),   // exp
		6: now.Unix(),      // iat
		65534: map[string]interface{}{
			"bits": 1,
			"lst":  base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(tokenStatusList.StatusList.Compressed()),
		},
	}

	claimsData, err := cbor.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	headers := cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm:   cose.AlgorithmES256,
			cose.HeaderLabelContentType: "application/statuslist+cwt",
		},
		Unprotected: cose.UnprotectedHeader{
			33: []interface{}{cert.Raw},
		},
	}

	signer, err := cose.NewSigner(cose.AlgorithmES256, privateKey)
	if err != nil {
		return nil, fmt.Errorf(ErrSignCWTFormat, err)
	}

	msg := &cose.Sign1Message{
		Headers: headers,
		Payload: claimsData,
	}

	if err = msg.Sign(rand.Reader, []byte{}, signer); err != nil {
		return nil, fmt.Errorf(ErrSignCWTFormat, err)
	}

	cwtData, err := cbor.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	return cwtData, nil
}

// GenerateIdentifierJWT creates a JWT for the identifier list
func (f *StatusListFormatter) GenerateIdentifierJWT(identifierList map[string]int, country, listURL, expiryDate string) (string, error) {
	if !f.config.ValidateCountry(country) {
		return "", fmt.Errorf(ErrCountryNotSupportedFormat, country, f.config.CountryCode)
	}

	privKeyPath, certPath := f.config.GetCertificatePaths()

	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return "", fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	issuer := f.config.ServiceURL
	if issuer == "" {
		return "", ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	expiry, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return "", fmt.Errorf("invalid expiry date: %w", err)
	}

	claims := IdentifierJWTClaims{
		IdentifierList: identifierList,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   listURL,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["typ"] = "statuslist+jwt"
	token.Header["x5c"] = []string{base64.StdEncoding.EncodeToString(cert.Raw)}

	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %v", err)
	}

	return tokenString, nil
}

// GenerateIdentifierCWT creates a CWT for the identifier list
func (f *StatusListFormatter) GenerateIdentifierCWT(identifierList map[string]int, country, listURL, expiryDate string) ([]byte, error) {
	if !f.config.ValidateCountry(country) {
		return nil, fmt.Errorf(ErrCountryNotSupportedFormat, country, f.config.CountryCode)
	}

	privKeyPath, certPath := f.config.GetCertificatePaths()

	privateKey, err := f.loadPrivateKey(privKeyPath)
	if err != nil {
		return nil, fmt.Errorf(ErrLoadPrivateKeyFormat, err)
	}

	cert, err := f.loadCertificate(certPath)
	if err != nil {
		return nil, fmt.Errorf(ErrLoadCertificateFormat, err)
	}

	now := time.Now()

	issuer := f.config.ServiceURL
	if issuer == "" {
		return nil, ErrServiceURLEmpty
	}
	issuer = strings.TrimSuffix(issuer, "/")

	expiry, err := time.Parse("2006-01-02", expiryDate)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry date: %w", err)
	}

	claims := map[interface{}]interface{}{
		1: issuer,        // iss
		2: listURL,       // sub
		4: expiry.Unix(), // exp
		6: now.Unix(),    // iat
		65534: map[string]interface{}{
			"bits":           1,
			"identifier_map": identifierList,
		},
	}

	claimsData, err := cbor.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	headers := cose.Headers{
		Protected: cose.ProtectedHeader{
			cose.HeaderLabelAlgorithm:   cose.AlgorithmES256,
			cose.HeaderLabelContentType: "application/statuslist+cwt",
		},
		Unprotected: cose.UnprotectedHeader{
			33: []interface{}{cert.Raw},
		},
	}

	signer, err := cose.NewSigner(cose.AlgorithmES256, privateKey)
	if err != nil {
		return nil, fmt.Errorf(ErrSignCWTFormat, err)
	}

	msg := &cose.Sign1Message{
		Headers: headers,
		Payload: claimsData,
	}

	if err = msg.Sign(rand.Reader, []byte{}, signer); err != nil {
		return nil, fmt.Errorf(ErrSignCWTFormat, err)
	}

	cwtData, err := cbor.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf(ErrEncodeCWTFormat, err)
	}

	return cwtData, nil
}

// loadPrivateKey loads a private key from file
func (f *StatusListFormatter) loadPrivateKey(keyPath string) (*ecdsa.PrivateKey, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Find the PEM block
	block, err := f.findPrivateKeyPEMBlock(keyData)
	if err != nil {
		return nil, err
	}

	// Check for encrypted keys
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		return nil, fmt.Errorf("PKCS#8 encrypted private keys require external decryption. Please decrypt the key first: openssl pkcs8 -in encrypted_key.pem -out decrypted_key.pem")
	}

	// Parse the key
	privateKey, err := f.parsePrivateKeyBytes(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Validate it's an ECDSA key
	ecdsaKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA private key, got type: %T", privateKey)
	}

	return ecdsaKey, nil
}

// findPrivateKeyPEMBlock finds the first private key PEM block in the data
func (f *StatusListFormatter) findPrivateKeyPEMBlock(keyData []byte) (*pem.Block, error) {
	var block *pem.Block
	remaining := keyData

	for {
		block, remaining = pem.Decode(remaining)
		if block == nil {
			return nil, fmt.Errorf("failed to find any PEM block in key file")
		}

		// Look for private key blocks
		if strings.Contains(block.Type, "PRIVATE KEY") {
			return block, nil
		}

		if len(remaining) == 0 {
			return nil, fmt.Errorf("no private key block found in PEM file")
		}
	}
}

// parsePrivateKeyBytes tries to parse private key bytes in various formats
func (f *StatusListFormatter) parsePrivateKeyBytes(keyBytes []byte) (interface{}, error) {
	// Try PKCS8 format first (most common format)
	privateKey, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err == nil {
		return privateKey, nil
	}

	// Try EC private key format
	privateKey, err = x509.ParseECPrivateKey(keyBytes)
	if err == nil {
		return privateKey, nil
	}

	// Check if it's an RSA key (not supported)
	if f.isRSAKey(keyBytes) {
		return nil, fmt.Errorf("RSA keys are not supported, need ECDSA key")
	}

	// Return the original EC parsing error
	return nil, fmt.Errorf("failed to parse private key: %v", err)
}

// isRSAKey checks if the key bytes represent an RSA key
func (f *StatusListFormatter) isRSAKey(keyBytes []byte) bool {
	_, err := x509.ParsePKCS1PrivateKey(keyBytes)
	return err == nil
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
