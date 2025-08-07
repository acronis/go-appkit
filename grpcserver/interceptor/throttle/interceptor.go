/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"github.com/acronis/go-appkit/internal/throttleconfig"
)

type serviceRoute[T any] struct {
	methodPattern string
	interceptor   T
	excluded      bool
}

type serviceRouteManager[T any] struct {
	exactRoutes              map[string]serviceRoute[T]
	descSortedPrefixedRoutes []serviceRoute[T]
}

func newServiceRouteManager[T any](routes []serviceRoute[T]) *serviceRouteManager[T] {
	exactRoutes := make(map[string]serviceRoute[T])
	var prefixedRoutes []serviceRoute[T]
	for _, route := range routes {
		if strings.HasSuffix(route.methodPattern, "*") {
			prefixedRoutes = append(prefixedRoutes, route)
			prefixedRoutes[len(prefixedRoutes)-1].methodPattern = strings.TrimSuffix(route.methodPattern, "*")
		} else {
			exactRoutes[route.methodPattern] = route
		}
	}
	// Sort prefixed routes in desc order.
	sort.SliceStable(prefixedRoutes, func(i, j int) bool {
		return prefixedRoutes[i].methodPattern > prefixedRoutes[j].methodPattern
	})
	return &serviceRouteManager[T]{exactRoutes, prefixedRoutes}
}

func (rm *serviceRouteManager[T]) SearchRoute(fullMethod string, excluded bool) (serviceRoute[T], bool) {
	if exactRoutes, ok := rm.exactRoutes[fullMethod]; ok {
		if exactRoutes.excluded == excluded {
			return exactRoutes, true
		}
	}
	for i := range rm.descSortedPrefixedRoutes {
		match := strings.HasPrefix(fullMethod, rm.descSortedPrefixedRoutes[i].methodPattern) &&
			rm.descSortedPrefixedRoutes[i].excluded == excluded
		if match {
			return rm.descSortedPrefixedRoutes[i], true
		}
	}
	return serviceRoute[T]{}, false
}

func (rm *serviceRouteManager[T]) SearchMatchedRouteForRequest(fullMethod string) (serviceRoute[T], bool) {
	if r, ok := rm.SearchRoute(fullMethod, true); ok {
		return r, false
	}
	return rm.SearchRoute(fullMethod, false)
}

func makeServiceRoutes[T any](
	cfg *Config,
	tags []string,
	constructor func(cfg *Config, rule *RuleConfig) ([]T, error),
	chain func(interceptors []T) T,
) ([]serviceRoute[T], error) {
	var routes []serviceRoute[T]
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]

		if len(tags) != 0 && !throttleconfig.CheckStringSlicesIntersect(tags, rule.Tags) {
			continue
		}

		interceptors, err := constructor(cfg, rule)
		if err != nil {
			return nil, err
		}
		if len(interceptors) == 0 {
			continue
		}

		for _, sm := range rule.ServiceMethods {
			routes = append(routes, serviceRoute[T]{
				methodPattern: sm,
				interceptor:   chain(interceptors),
				excluded:      false,
			})
		}
		for _, sm := range rule.ExcludedServiceMethods {
			routes = append(routes, serviceRoute[T]{
				methodPattern: sm,
				interceptor:   chain(interceptors),
				excluded:      true,
			})
		}
	}

	return routes, nil
}

func getKeyFromHeader(ctx context.Context, headerName string, noBypassEmpty bool) (key string, bypass bool, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", !noBypassEmpty, nil
	}
	values := md.Get(strings.ToLower(headerName))
	if len(values) == 0 {
		return "", !noBypassEmpty, nil
	}
	headerVal := strings.TrimSpace(values[0])
	if noBypassEmpty {
		return headerVal, false, nil
	}
	return headerVal, headerVal == "", nil
}

func getKeyFromRemoteAddr(ctx context.Context) (key string, bypass bool, err error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", false, fmt.Errorf("unable to get peer from context")
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return "", false, fmt.Errorf("split host and port from peer address: %w", err)
	}
	return host, false, nil
}
