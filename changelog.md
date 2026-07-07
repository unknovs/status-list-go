# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

* **BREAKING (CWT status list path only):** `GenerateCWT` now emits an IETF
  [draft-ietf-oauth-status-list-12](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-status-list-12)
  conformant Status List Token. Five wire changes, all confined to the
  previously-unconsumed CWT path (no known consumer, hence "breaking" only in
  theory):
  * `status_list` claim moved from private CWT claim key `65534` to `65533`;
  * its `lst` member is now a raw CBOR byte string (was a base64 text string);
  * a `ttl` claim (key `65534`, 3600s) was added so verifiers can cache the token;
  * the COSE token-type indicator is now the protected-header `typ` label `16`
    (RFC 9596, value `application/statuslist+cwt`) instead of content-type label `3`;
  * `x5chain` (label `33`) moved into the COSE protected header.

### Added

* `GenerateJWT`: additive top-level `ttl` claim (3600s) for verifier caching.
  Purely additive — the rest of the JWT wire shape (`sub`/`iat`/`exp`,
  `status_list.bits`, unpadded base64url `lst`, `statuslist+jwt` typ header) is
  unchanged, so there is no break for existing JWT consumers.
* Dependency on the EU reference verifier library
  [`github.com/gmb-eudi/go-statuslist`](https://github.com/gmb-eudi/go-statuslist)
  plus a conformance regression gate
  (`services.TestIssuedTokensVerifyWithGoStatuslist`) that runs every issued
  ASL-JWT and ASL-CWT through that library, so any future drift out of
  draft-ietf-oauth-status-list-12 fails the build. Interop fixtures generated
  once from these formatters are also committed to and re-verified by
  go-statuslist's own `interop_test.go` (cross-repo regression net).

### Notes

* The ARL / identifier-list mechanism (`GenerateIdentifierJWT` /
  `GenerateIdentifierCWT`) is intentionally left unchanged and remains NOT
  conformant to draft-ietf-oauth-status-list-12, pending the ARF VCR_11
  Commission Implementing/Technical Specification (out of scope, decision D-2).

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
