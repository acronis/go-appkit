/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

// Package ratelimit provides rate limiting functionality to control the rate
// at which requests are processed over time.
//
// The package implements a generic RequestProcessor that can be used with both HTTP
// and gRPC requests to enforce rate limits with optional backlog queuing.
// When rate limits are exceeded, requests can be queued in a backlog with
// configurable timeout, or rejected immediately if the backlog is full.
//
// Key features:
//   - Token bucket and sliding window rate limiting algorithms
//   - Configurable rate limits per key or globally
//   - Optional backlog queuing with timeout and retry-after headers
//   - LRU-based key management for memory efficiency
//   - Integration with external rate limiters via Limiter interface
package ratelimit
