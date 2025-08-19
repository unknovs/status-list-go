# status-list-go

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)

*A Go-based revocation project for Attestation Status List and Attestation Revocation List.*

## Description

`github.com/unknovs/status-list-go` is a Go service designed to issue and manage revocation status using Attestation Status Lists and Attestation Revocation Lists.

## Cryptographic Keys and Certificates

### Requirements

The Status List service requires ECDSA P-256 (prime256v1) cryptographic keys and certificates for JWT signing. The service supports both development and production deployment scenarios.

### Key Formats Supported

**Private Keys:**

- **Unencrypted PEM format**

**Certificates:**

- **DER format** (binary)
- **PEM format** (base64-encoded)

### Configuration

The service uses environment variables for flexible certificate management:

```bash
# Certificate paths (can be absolute or relative)
PRIVATE_KEY_PATH=/path/to/private-key.pem
CERTIFICATE_PATH=/path/to/certificate.der
COUNTRY_CODE=LV

# API configuration
API_KEY=your-api-key
SERVICE_URL=http://localhost:8080/
```

### Development Setup

For development, you can generate test certificates:

```bash
# Generate a private key
openssl ecparam -genkey -name prime256v1 -out private-key.pem

# Generate a self-signed certificate
openssl req -new -x509 -key private-key.pem -out certificate.pem -days 365

# Convert certificate to DER format (optional)
openssl x509 -in certificate.pem -outform DER -out certificate.der
```

### Production Deployment with Docker Secrets

For production, use Docker secrets or mounted volumes:

```bash
# Using Docker secrets
echo "your-private-key-content" | docker secret create private_key -
echo "your-certificate-content" | docker secret create certificate -

docker run -d \
  --secret private_key \
  --secret certificate \
  -e PRIVATE_KEY_PATH=/run/secrets/private_key \
  -e CERTIFICATE_PATH=/run/secrets/certificate \
  -e COUNTRY_CODE=LV \
  -e API_KEY=your-production-api-key \
  your-statuslist-service
```

### Key Conversion

If you have an encrypted PKCS#8 private key, you can convert it to an unencrypted format:

```bash
# Convert encrypted PKCS#8 to unencrypted PEM
openssl pkcs8 -in encrypted-key.pem -out decrypted-key.pem -passin pass:your-password

# Verify the key format
openssl pkey -in decrypted-key.pem -text -noout
```

### Security Considerations

- **Production**: Always use properly issued certificates from a trusted CA
- **Key Protection**: Keep private keys secure and use appropriate file permissions (600)
- **Encryption**: For production, consider using encrypted keys with secure password management
- **Rotation**: Implement regular key rotation procedures
- **Validation**: Ensure certificates have proper key usage extensions for digital signatures

### Troubleshooting

**Common Issues:**

1. **"failed to parse private key" errors:**
   - Ensure your private key is in ECDSA P-256 format
   - Convert encrypted PKCS#8 keys using the conversion command above

2. **"ASN.1 structure error" messages:**
   - This typically indicates an encrypted key that couldn't be decrypted
   - Verify the password is correct
   - Convert to unencrypted PEM format for development

3. **"Certificate loading failed":**
   - Verify the certificate file exists and is readable
   - Ensure the certificate matches your private key
   - Both DER and PEM formats are supported

4. **JWT signing failures:**
   - Confirm the private key and certificate are from the same key pair
   - Verify the certificate is valid and not expired

## Swagger

```sh
HOST{/token_status_list/swagger}
```

## Features

- Supports Attestation Status List (ASL) [draft-ietf-oauth-status-list-02](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-status-list-02)
- Supports Attestation Revocation List (ARL) [ ISO/IEC CD 18013-5 second edition ]

## How to contribute

We welcome contributions to this project. To ensure that the process is smooth for everyone
involved, follow the guidelines found in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

### License details

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
