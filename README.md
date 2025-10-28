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

## Storage Backend Configuration

The Status List service supports pluggable storage backends for horizontal scaling and flexibility.

### Available Storage Backends

#### 1. Local Filesystem (Default)

The default storage backend uses the local filesystem. This is suitable for:
- Development and testing
- Single-instance deployments
- Environments where shared storage is not required

**Configuration:**

```bash
# Local storage is the default - no additional configuration needed
STATUS_LIST_STORAGE=local  # Optional, defaults to "local"
STATUS_LIST_DIR=/var/opt/status_lists
BACKUP_DIR=/var/opt/status_list_backup
LOG_DIR=/tmp/status_lists
```

#### 2. S3 / S3-Compatible Storage

For production deployments requiring horizontal scaling with multiple service instances, use S3 or S3-compatible storage (MinIO, LocalStack, etc.).

**Configuration:**

```bash
# S3 Storage Backend
STATUS_LIST_STORAGE=s3
S3_BUCKET=status-lists
S3_REGION=us-east-1  # Optional for S3-compatible services
S3_ACCESS_KEY_ID=your-access-key
S3_SECRET_ACCESS_KEY=your-secret-key
S3_ENDPOINT=http://localhost:9000  # Optional: for MinIO or other S3-compatible services
```

**AWS S3 Example:**

```bash
STATUS_LIST_STORAGE=s3
S3_BUCKET=my-status-lists-bucket
S3_REGION=eu-west-1
S3_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
S3_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**MinIO Example (Local Development):**

```bash
STATUS_LIST_STORAGE=s3
S3_BUCKET=status-lists
S3_ENDPOINT=http://minio:9000
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_REGION=us-east-1
```

### Docker Compose with S3 Storage

The included `docker-compose.yml` provides a complete setup with MinIO for local S3 testing:

```yaml
# Start with MinIO S3 storage
docker-compose up -d

# The minio-init service automatically creates the required bucket
# MinIO console available at: http://localhost:9001
```

To enable S3 storage in the service, uncomment the S3 environment variables in `docker-compose.yml`:

```yaml
environment:
  STATUS_LIST_STORAGE: "s3"
  S3_BUCKET: "status-lists"
  S3_ENDPOINT: "http://minio:9000"
  S3_ACCESS_KEY_ID: "minioadmin"
  S3_SECRET_ACCESS_KEY: "minioadmin"
  S3_REGION: "us-east-1"
```

### Storage Features

- **Optimistic Locking**: Version-based concurrency control prevents data corruption
- **Atomic Operations**: All write operations are atomic (local: temp file + rename, S3: object metadata versioning)
- **Automatic Retry**: AWS SDK built-in exponential backoff for transient failures
- **Connection Validation**: S3 bucket accessibility is validated at startup

### Multi-Instance Deployment

When using S3 storage, multiple service instances can share the same bucket:

```bash
# Instance A
docker run -d -p 8080:8080 \
  -e STATUS_LIST_STORAGE=s3 \
  -e S3_BUCKET=shared-status-lists \
  statuslist-service

# Instance B (different port)
docker run -d -p 8081:8080 \
  -e STATUS_LIST_STORAGE=s3 \
  -e S3_BUCKET=shared-status-lists \
  statuslist-service
```

Both instances will read and write to the same S3 bucket, enabling horizontal scaling.

## Swagger

```sh
HOST{/token_status_list/swagger}
```

## Features

- Supports Attestation Status List (ASL) [draft-ietf-oauth-status-list-02](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-status-list-02)
  * need to check diferences with draft v12
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
