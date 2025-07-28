/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package inflightlimit

import (
	"context"
	"fmt"
	"time"

	"github.com/acronis/go-appkit/lrucache"
)

// DefaultInFlightLimitBacklogTimeout determines the default timeout for backlog processing.
const DefaultInFlightLimitBacklogTimeout = time.Second * 5

// Params contains common data that relates to the in-flight limiting procedure.
type Params struct {
	Key               string
	RequestBacklogged bool
}

// RequestHandler abstracts the common operations for both HTTP and gRPC requests.
type RequestHandler interface {
	// GetContext returns the request context.
	GetContext() context.Context

	// GetKey extracts the in-flight limiting key from the request.
	// Returns key, bypass (whether to bypass in-flight limiting), and error.
	GetKey() (string, bool, error)

	// Execute processes the actual request.
	Execute() error

	// OnReject handles request rejection when in-flight limit is exceeded.
	OnReject(params Params) error

	// OnError handles errors that occur during in-flight limiting.
	OnError(params Params, err error) error
}

// RequestProcessor handles the common in-flight limiting logic for any request type.
type RequestProcessor struct {
	limit          int
	getSlots       slotsProvider
	backlogTimeout time.Duration
	dryRun         bool
}

// BacklogParams defines parameters for the backlog processing.
type BacklogParams struct {
	MaxKeys int
	Limit   int
	Timeout time.Duration
}

// slotsProvider provides in-flight and backlog slots for limiting.
type slotsProvider func(key string) (inFlightSlots chan struct{}, backlogSlots chan struct{})

// NewRequestProcessor creates a new generic in-flight request processor.
func NewRequestProcessor(limit int, backlogParams BacklogParams, dryRun bool) (*RequestProcessor, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit should be positive, got %d", limit)
	}
	if backlogParams.Limit < 0 {
		return nil, fmt.Errorf("backlog limit should not be negative, got %d", backlogParams.Limit)
	}
	if backlogParams.MaxKeys < 0 {
		return nil, fmt.Errorf("max keys for backlog should not be negative, got %d", backlogParams.MaxKeys)
	}

	getSlots, err := newSlotsProvider(limit, backlogParams.Limit, backlogParams.MaxKeys)
	if err != nil {
		return nil, fmt.Errorf("create slots provider: %w", err)
	}

	if backlogParams.Timeout == 0 {
		backlogParams.Timeout = DefaultInFlightLimitBacklogTimeout
	}

	return &RequestProcessor{
		limit:          limit,
		getSlots:       getSlots,
		backlogTimeout: backlogParams.Timeout,
		dryRun:         dryRun,
	}, nil
}

// ProcessRequest contains the shared in-flight limiting logic.
func (p *RequestProcessor) ProcessRequest(rh RequestHandler) error {
	key, bypass, err := rh.GetKey()
	if err != nil {
		return rh.OnError(Params{Key: key}, fmt.Errorf("get key for in-flight limit: %w", err))
	}
	if bypass {
		return rh.Execute()
	}

	slots, backlogSlots := p.getSlots(key)

	backlogged := false
	defer func() {
		if backlogged {
			select {
			case <-backlogSlots:
			default:
			}
		}
	}()

	select {
	case backlogSlots <- struct{}{}:
		backlogged = true
	default:
		return rh.OnReject(Params{Key: key, RequestBacklogged: false})
	}

	return p.processBackloggedRequest(rh, key, slots)
}

// processBackloggedRequest handles a request that has been placed in the backlog queue.
func (p *RequestProcessor) processBackloggedRequest(rh RequestHandler, key string, slots chan struct{}) error {
	acquired := false
	defer func() {
		if acquired {
			select {
			case <-slots:
			default:
			}
		}
	}()

	if p.dryRun {
		select {
		case slots <- struct{}{}:
			acquired = true
			return rh.Execute()
		default:
			return rh.OnReject(Params{Key: key, RequestBacklogged: true})
		}
	}

	ctx := rh.GetContext()
	backlogTimeoutTimer := time.NewTimer(p.backlogTimeout)
	defer backlogTimeoutTimer.Stop()

	select {
	case slots <- struct{}{}:
		acquired = true
		return rh.Execute()
	case <-backlogTimeoutTimer.C:
		return rh.OnReject(Params{Key: key, RequestBacklogged: true})
	case <-ctx.Done():
		return rh.OnError(Params{Key: key, RequestBacklogged: true}, ctx.Err())
	}
}

// newSlotsProvider creates a new slots provider for in-flight and backlog limiting.
func newSlotsProvider(limit, backlogLimit, maxKeys int) (slotsProvider, error) {
	if maxKeys == 0 {
		slots := make(chan struct{}, limit)
		backlogSlots := make(chan struct{}, limit+backlogLimit)
		return func(key string) (chan struct{}, chan struct{}) {
			return slots, backlogSlots
		}, nil
	}

	type KeysZoneItem struct {
		slots        chan struct{}
		backlogSlots chan struct{}
	}

	keysZone, err := lrucache.New[string, *KeysZoneItem](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return func(key string) (chan struct{}, chan struct{}) {
		keysZoneItem, _ := keysZone.GetOrAdd(key, func() *KeysZoneItem {
			return &KeysZoneItem{
				slots:        make(chan struct{}, limit),
				backlogSlots: make(chan struct{}, limit+backlogLimit),
			}
		})
		return keysZoneItem.slots, keysZoneItem.backlogSlots
	}, nil
}
