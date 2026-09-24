// Copyright (c) 2015-Present CloudFoundry.org Foundation, Inc. All Rights Reserved.
//
// This file is adapted from code.cloudfoundry.org/bytefmt (Apache License 2.0).
// It contains only the formatting and parsing functions used by go-appkit.

package bytefmt

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

const (
	byteUnit = 1 << (10 * iota)
	kilobyte
	megabyte
	gigabyte
	terabyte
	petabyte
	exabyte
)

var invalidByteQuantityError = errors.New("byte quantity must be a positive integer with a unit of measurement like M, MB, MiB, G, GiB, or GB")

// ByteSize formats a byte count using the largest binary unit it contains.
func ByteSize(bytes uint64) string {
	unit := ""
	value := float64(bytes)

	switch {
	case bytes >= exabyte:
		unit = "E"
		value /= exabyte
	case bytes >= petabyte:
		unit = "P"
		value /= petabyte
	case bytes >= terabyte:
		unit = "T"
		value /= terabyte
	case bytes >= gigabyte:
		unit = "G"
		value /= gigabyte
	case bytes >= megabyte:
		unit = "M"
		value /= megabyte
	case bytes >= kilobyte:
		unit = "K"
		value /= kilobyte
	case bytes >= byteUnit:
		unit = "B"
	case bytes == 0:
		return "0B"
	}

	result := strconv.FormatFloat(value, 'f', 1, 64)
	return strings.TrimSuffix(result, ".0") + unit
}

// ToBytes parses a byte quantity. K, KB, and KiB (and their larger units)
// all use powers of 1024, matching the configuration formats used here.
func ToBytes(s string) (uint64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	i := strings.IndexFunc(s, unicode.IsLetter)
	if i == -1 {
		return 0, invalidByteQuantityError
	}

	bytes, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || bytes < 0 {
		return 0, invalidByteQuantityError
	}

	switch s[i:] {
	case "E", "EB", "EIB":
		return uint64(bytes * exabyte), nil
	case "P", "PB", "PIB":
		return uint64(bytes * petabyte), nil
	case "T", "TB", "TIB":
		return uint64(bytes * terabyte), nil
	case "G", "GB", "GIB":
		return uint64(bytes * gigabyte), nil
	case "M", "MB", "MIB":
		return uint64(bytes * megabyte), nil
	case "K", "KB", "KIB":
		return uint64(bytes * kilobyte), nil
	case "B":
		return uint64(bytes), nil
	default:
		return 0, invalidByteQuantityError
	}
}
