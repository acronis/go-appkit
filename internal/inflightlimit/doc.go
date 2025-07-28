/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

// Package inflightlimit provides in-flight request limiting functionality to control
// the number of concurrent requests being processed at any given time.
//
// The package implements a generic RequestProcessor that can be used with both HTTP
// and gRPC requests to enforce in-flight limits with optional backlog queuing.
// When the in-flight limit is reached, requests can be queued in a backlog with
// configurable timeout, or rejected immediately if the backlog is full.
//
// Key features:
//   - Configurable in-flight limits per key or globally
//   - Optional backlog queuing with timeout
//   - Dry-run mode for testing
//   - LRU-based key management for memory efficiency
package inflightlimit
