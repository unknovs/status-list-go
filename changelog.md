# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

* Initial Go implementation
* REST API endpoints for status list management (/take, /get, /set)
* JWT and CWT format support for status lists and identifier lists
* Automatic list renewal background process
* Docker support
* Comprehensive configuration management
* Swagger API documentation

### Features

* **Status List Management**: Create and manage attestation status lists
* **Identifier List Management**: Create and manage attestation identifier lists
* **Multiple Format Support**: JWT and CWT formats for both status and identifier lists
* **Cryptographic Signing**: ECDSA signing with country-specific certificates
* **Background Renewal**: Automatic renewal of lists at scheduled intervals
* **API Authentication**: X-API-Key based authentication
* **File-based Storage**: Persistent storage of lists on disk
* **Backup System**: Automatic backup creation during renewal
* **Docker Support**: Containerized deployment
* **Environment Configuration**: Flexible configuration via environment variables

### Technical Details

* Built with Go 1.21+
* Uses Gin web framework for HTTP routing
* JWT handling with golang-jwt/jwt library
* CBOR support for CWT format
* Concurrent-safe list management
* Structured logging
* Health check endpoint
* CORS support
