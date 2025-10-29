/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package middleware

// MetricsParams stores parameters for the HTTPRequestMetrics middleware
// that may be modified dynamically by the other underlying middlewares/handlers.
type MetricsParams struct {
	values map[string]string
}

// SetValue adds a new label with the specified name and value.
// If a label with the same name already exists, it will be overwritten.
func (mp *MetricsParams) SetValue(name, value string) {
	if mp.values == nil {
		mp.values = make(map[string]string)
	}
	mp.values[name] = value
}
