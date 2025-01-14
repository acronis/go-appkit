/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import "reflect"

// Config is a common interface for configuration objects that may be used by Loader.
type Config interface {
	SetProviderDefaults(dp DataProvider)
	Set(dp DataProvider) error
}

// KeyPrefixProvider is an interface for providing key prefix that will be used for configuration parameters.
type KeyPrefixProvider interface {
	KeyPrefix() string
}

// CallSetProviderDefaultsForFields finds all initialized (non-nil) fields of the passed object
// that implement Config interface and calls SetProviderDefaults() method for each of them.
func CallSetProviderDefaultsForFields(obj interface{}, dp DataProvider) {
	el := reflect.ValueOf(obj).Elem()
	for i := 0; i < el.NumField(); i++ {
		if !el.Type().Field(i).IsExported() {
			continue
		}
		v := el.Field(i).Interface()
		if reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() {
			continue
		}
		if c, ok := v.(Config); ok {
			cDp := dp
			if kpDp, ok := v.(KeyPrefixProvider); ok && kpDp.KeyPrefix() != "" {
				cDp = NewKeyPrefixedDataProvider(dp, kpDp.KeyPrefix())
			}
			c.SetProviderDefaults(cDp)
		}
	}
}

// CallSetForFields finds all initialized (non-nil) fields of the passed object
// that implement Config interface and calls Set() method for each of them.
func CallSetForFields(obj interface{}, dp DataProvider) error {
	el := reflect.ValueOf(obj).Elem()
	for i := 0; i < el.NumField(); i++ {
		if !el.Type().Field(i).IsExported() {
			continue
		}
		v := el.Field(i).Interface()
		if reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() {
			continue
		}
		if c, ok := v.(Config); ok {
			cDp := dp
			if kpDp, ok := v.(KeyPrefixProvider); ok && kpDp.KeyPrefix() != "" {
				cDp = NewKeyPrefixedDataProvider(dp, kpDp.KeyPrefix())
			}
			if err := c.Set(cDp); err != nil {
				return err
			}
		}
	}
	return nil
}
