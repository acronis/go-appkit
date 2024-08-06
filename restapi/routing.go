/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
)

// RoutePath represents route's path.
type RoutePath struct {
	Raw            string
	NormalizedPath string
	RegExpPath     *regexp.Regexp
	ExactMatch     bool
	ForwardMatch   bool
}

// ParseRoutePath parses string representation of route's path.
// Syntax: [ = | ~ | ^~ ] urlPath
// Semantic for modifier is used the same as in Nginx (https://nginx.org/en/docs/http/ngx_http_core_module.html#location).
func ParseRoutePath(rp string) (RoutePath, error) {
	rp = strings.TrimSpace(rp)
	if rp == "" {
		return RoutePath{}, fmt.Errorf("path is missing")
	}
	switch {
	case strings.HasPrefix(rp, "="):
		p := strings.TrimSpace(rp[1:])
		if !strings.HasPrefix(p, "/") {
			return RoutePath{}, fmt.Errorf("path should be started with \"/\" in case of exact matching")
		}
		return RoutePath{Raw: rp, NormalizedPath: NormalizeURLPath(p), ExactMatch: true}, nil

	case strings.HasPrefix(rp, "^~"):
		p := strings.TrimSpace(rp[2:])
		if !strings.HasPrefix(p, "/") {
			return RoutePath{}, fmt.Errorf("path should be started with \"/\" in case of forward matching")
		}
		return RoutePath{Raw: rp, NormalizedPath: NormalizeURLPath(p), ForwardMatch: true}, nil

	case strings.HasPrefix(rp, "~"):
		p := strings.TrimSpace(rp[1:])
		if p == "" {
			return RoutePath{}, fmt.Errorf("regular expression is missing")
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return RoutePath{}, err
		}
		return RoutePath{Raw: rp, RegExpPath: re}, nil
	}

	if !strings.HasPrefix(rp, "/") {
		return RoutePath{}, fmt.Errorf("path should be started with \"/\" in case of prefixed matching")
	}
	return RoutePath{Raw: rp, NormalizedPath: NormalizeURLPath(rp)}, nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (rp *RoutePath) UnmarshalText(text []byte) (err error) {
	*rp, err = ParseRoutePath(string(text))
	return
}

// Route represents route for handling.
type Route struct {
	Path        RoutePath
	Methods     []string
	Handler     http.Handler
	Middlewares []func(http.Handler) http.Handler
	Excluded    bool // Set to true for routes that are matched to be excluded.
}

// NewRoute returns a new route.
func NewRoute(cfg RouteConfig, handler http.Handler, middlewares []func(http.Handler) http.Handler) Route {
	return Route{
		Path:        cfg.Path,
		Methods:     cfg.MethodsInUpperCase(),
		Handler:     handler,
		Middlewares: middlewares,
	}
}

// NewExcludedRoute returns a new route that will be used as exclusion in matching.
func NewExcludedRoute(cfg RouteConfig) Route {
	return Route{
		Path:     cfg.Path,
		Methods:  cfg.MethodsInUpperCase(),
		Excluded: true,
	}
}

// RoutesManager contains routes for handling and allows to search among them.
type RoutesManager struct {
	exactRoutes              map[string][]Route
	descSortedPrefixedRoutes []Route
	regExpRoutes             []Route
}

// NewRoutesManager create new RoutesManager.
func NewRoutesManager(routes []Route) *RoutesManager {
	exactRoutes := make(map[string][]Route)
	var prefixedRoutes []Route
	var regExpRoutes []Route
	for _, route := range routes {
		switch {
		case route.Path.ExactMatch:
			exactRoutes[route.Path.NormalizedPath] = append(exactRoutes[route.Path.NormalizedPath], route)
		case route.Path.RegExpPath != nil:
			regExpRoutes = append(regExpRoutes, route)
		default:
			prefixedRoutes = append(prefixedRoutes, route)
		}
	}

	// For further searching in each slice first must go routes for which methods are specified.
	// That's why we sort all slices here.

	for i := range exactRoutes {
		pathRoutes := exactRoutes[i]
		sort.SliceStable(pathRoutes, func(i, j int) bool {
			return len(pathRoutes[i].Methods) != 0 && len(pathRoutes[j].Methods) == 0
		})
	}

	// Sort prefixed routes in desc order.
	sort.SliceStable(prefixedRoutes, func(i, j int) bool {
		if prefixedRoutes[i].Path.NormalizedPath == prefixedRoutes[j].Path.NormalizedPath {
			return len(prefixedRoutes[i].Methods) != 0 && len(prefixedRoutes[j].Methods) == 0
		}
		return prefixedRoutes[i].Path.NormalizedPath > prefixedRoutes[j].Path.NormalizedPath
	})

	sort.SliceStable(regExpRoutes, func(i, j int) bool {
		return len(regExpRoutes[i].Methods) != 0 && len(regExpRoutes[j].Methods) == 0
	})

	return &RoutesManager{exactRoutes, prefixedRoutes, regExpRoutes}
}

// SearchMatchedRouteForRequest searches Route that matches the passing http.Request.
// Algorithm is the same as used in Nginx for locations matching (https://nginx.org/en/docs/http/ngx_http_core_module.html#location).
// Excluded routes has priority.
func (r *RoutesManager) SearchMatchedRouteForRequest(req *http.Request) (Route, bool) {
	normalizedReqURLPath := NormalizeURLPath(req.URL.Path)
	if r, ok := r.SearchRoute(normalizedReqURLPath, req.Method, true); ok {
		return r, false
	}
	return r.SearchRoute(normalizedReqURLPath, req.Method, false)
}

// SearchRoute searches Route by passed path and method.
// Path should be normalized (see NormalizeURLPath for this).
// If the excluded arg is true, search will be done only among excluded routes. If false - only among included routes.
// nolint:gocyclo
func (r *RoutesManager) SearchRoute(normalizedPath string, method string, excluded bool) (Route, bool) {
	reqMethodMatchesRoute := func(route *Route) bool {
		if len(route.Methods) == 0 {
			return true
		}
		for i := range route.Methods {
			if route.Methods[i] == method {
				return true
			}
		}
		return false
	}

	if exactRoutes, ok := r.exactRoutes[normalizedPath]; ok {
		for i := range exactRoutes {
			if exactRoutes[i].Excluded == excluded && reqMethodMatchesRoute(&exactRoutes[i]) {
				return exactRoutes[i], true
			}
		}
	}

	var longestPrefixedRoute *Route
	for i := range r.descSortedPrefixedRoutes {
		match := strings.HasPrefix(normalizedPath, r.descSortedPrefixedRoutes[i].Path.NormalizedPath) &&
			r.descSortedPrefixedRoutes[i].Excluded == excluded &&
			reqMethodMatchesRoute(&r.descSortedPrefixedRoutes[i])
		if match {
			longestPrefixedRoute = &r.descSortedPrefixedRoutes[i]
			break
		}
	}
	if longestPrefixedRoute != nil && longestPrefixedRoute.Path.ForwardMatch {
		return *longestPrefixedRoute, true
	}

	for i := range r.regExpRoutes {
		match := r.regExpRoutes[i].Path.RegExpPath.MatchString(normalizedPath) &&
			r.regExpRoutes[i].Excluded == excluded &&
			reqMethodMatchesRoute(&r.regExpRoutes[i])
		if match {
			return r.regExpRoutes[i], true
		}
	}

	if longestPrefixedRoute != nil {
		return *longestPrefixedRoute, true
	}

	return Route{}, false
}

// RouteConfig represents route's configuration.
type RouteConfig struct {
	// Path is a struct that contains info about route path.
	// ParseRoutePath function should be used for constructing it from the string representation.
	Path RoutePath `mapstructure:"path"`

	// Methods is a list of case-insensitive HTTP verbs/methods.
	Methods []string `mapstructure:"methods"`
}

// MethodsInUpperCase returns list of route's methods in upper-case.
func (r *RouteConfig) MethodsInUpperCase() []string {
	upperMethods := make([]string, 0, len(r.Methods))
	for _, m := range r.Methods {
		upperMethods = append(upperMethods, strings.ToUpper(m))
	}
	return upperMethods
}

var availableHTTPMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}

// Validate validates RouteConfig
func (r *RouteConfig) Validate() error {
	validateMethod := func(method string) error {
		for _, am := range availableHTTPMethods {
			if method == am {
				return nil
			}
		}
		return fmt.Errorf("unknown method %q", method)
	}

	if r.Path.Raw == "" {
		return fmt.Errorf("path is missing")
	}
	for _, method := range r.MethodsInUpperCase() {
		if err := validateMethod(method); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeURLPath normalizes URL path (i.e. for example, it convert /foo///bar/.. to /foo).
func NormalizeURLPath(urlPath string) string {
	res := path.Clean("/" + urlPath)
	if strings.HasSuffix(urlPath, "/") && res != "/" {
		res += "/"
	}
	return res
}
