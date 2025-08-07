/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

// Package throttle provides configurable gRPC interceptors for rate limiting and in-flight request limiting.
//
// This package implements two main interceptors:
//   - UnaryInterceptor: For unary gRPC calls
//   - StreamInterceptor: For streaming gRPC calls
//
// Both interceptors support:
//   - Rate limiting with configurable algorithms (leaky bucket, sliding window)
//   - In-flight request limiting
//   - Service method pattern matching
//   - Identity-based, header-based, and remote address-based key extraction
//   - Dry-run mode for testing
//   - Comprehensive configuration through JSON/YAML
package throttle
