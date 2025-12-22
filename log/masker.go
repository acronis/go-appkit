package log

import (
	"regexp"
	"strings"
	"unsafe"

	"github.com/cloudflare/ahocorasick"
)

// Mask is used to mask a secret in strings.
type Mask struct {
	RegExp *regexp.Regexp
	Mask   string
}

func NewMask(cfg MaskConfig) Mask {
	return Mask{regexp.MustCompile(cfg.RegExp), cfg.Mask}
}

// FieldMasker is used to mask a field in different formats.
type FieldMasker struct {
	Field string // Field is a name of a field used in RegExp, must be lowercase
	Masks []Mask
}

const ignoreCase = `(?i)`

func NewFieldMasker(cfg MaskingRuleConfig) FieldMasker {
	fMask := FieldMasker{Field: strings.ToLower(cfg.Field),
		Masks: make([]Mask, 0, len(cfg.Masks)),
	}

	for _, repCfg := range cfg.Masks {
		fMask.Masks = append(fMask.Masks, NewMask(repCfg))
	}
	for _, format := range cfg.Formats {
		switch format {
		case FieldMaskFormatHTTPHeader:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{ignoreCase + cfg.Field + `: .+?\r\n`, cfg.Field + ": ***\r\n"}))
		case FieldMaskFormatJSON:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{ignoreCase + `"` + cfg.Field + `"\s*:\s*".*?[^\\]"`, `"` + cfg.Field + `": "***"`}))
		case FieldMaskFormatURLEncoded:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{ignoreCase + cfg.Field + `\s*=\s*[^&\s]+`, cfg.Field + "=***"}))
		}
	}
	return fMask
}

// Masker uses the Aho-Corasick algorithm to simultaneously search for all patterns in a string.
// The order of applying masking rules is guaranteed.
type Masker struct {
	fieldMasks       []FieldMasker
	aho              *ahocorasick.Matcher
	maskFunc         func(s string) string
	matchToRuleIndex []int // maps Aho-Corasick match indices to rule indices
	ruleToMatchIdx   []int // maps field mask indices to Aho-Corasick match indices
}

// limitation for Aho-Corasick algorithm version of Mask function
const maxRulesForOpt = 64

// NewMasker creates a new Masker instance.
// This function initializes two mappings without memory barriers because we are confident
// that the function has "happens-before" guarantee.
// Otherwise, we would need to use redundant mutex operations in Mask().
func NewMasker(rules []MaskingRuleConfig) *Masker {
	r := &Masker{
		fieldMasks:       make([]FieldMasker, 0, len(rules)),
		matchToRuleIndex: make([]int, 0),
		ruleToMatchIdx:   make([]int, len(rules)),
	}

	patterns := make([]string, 0, len(rules))
	for idx, rule := range rules {
		masker := NewFieldMasker(rule)
		r.fieldMasks = append(r.fieldMasks, masker)
		if masker.Field == "" {
			// assign -1 to indicate that no specific match is needed (empty fields masks apply universally)
			r.ruleToMatchIdx[idx] = -1
		} else {
			patterns = append(patterns, strings.ToLower(rule.Field))
			// map the current rule index to the next available match index
			r.matchToRuleIndex = append(r.matchToRuleIndex, idx)
			// store the mapping from the rule index to the corresponding match index
			r.ruleToMatchIdx[idx] = len(r.matchToRuleIndex) - 1
		}
	}
	r.aho = ahocorasick.NewStringMatcher(patterns)

	if len(patterns) <= maxRulesForOpt {
		r.maskFunc = r.maskFor64Fields
	} else {
		r.maskFunc = r.mask
	}

	return r
}

func stringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s)) //nolint:gosec // reduce number of allocations
}

// Mask applies the appropriate masking function to the input string.
func (r *Masker) Mask(s string) string {
	return r.maskFunc(s)
}

func (r *Masker) mask(s string) string {
	lower := stringToBytes(strings.ToLower(s))
	matches := r.aho.MatchThreadSafe(lower)

	matched := make([]bool, len(r.matchToRuleIndex))
	for _, matchIdx := range matches {
		// for each match index from Aho-Corasick, find the corresponding rule index
		ruleIdx := r.matchToRuleIndex[matchIdx]
		matched[ruleIdx] = true
	}

	for i, fieldMask := range r.fieldMasks {
		matchIdx := r.ruleToMatchIdx[i]
		apply := false
		if matchIdx == -1 {
			apply = true
		} else {
			// get the index in matchToRuleIndex corresponding to this field
			apply = matched[matchIdx]
		}

		if apply {
			for _, rep := range fieldMask.Masks {
				s = rep.RegExp.ReplaceAllString(s, rep.Mask)
			}
		}
	}

	return s
}

// maskFor64Fields is a optimized version of mask for 64 fields max.
func (r *Masker) maskFor64Fields(s string) string {
	lower := stringToBytes(strings.ToLower(s))
	matches := r.aho.MatchThreadSafe(lower)

	var matchedMask uint64 // 64 fields max
	for _, idx := range matches {
		matchedMask |= 1 << idx
	}

	for i, fieldMask := range r.fieldMasks {
		matchIdx := r.ruleToMatchIdx[i]
		apply := false
		if matchIdx == -1 {
			apply = true
		} else {
			apply = (matchedMask & (1 << matchIdx)) != 0
		}

		if apply {
			for _, rep := range fieldMask.Masks {
				s = rep.RegExp.ReplaceAllString(s, rep.Mask)
			}
		}
	}
	return s
}

var DefaultMasks = []MaskingRuleConfig{
	{
		Field:   "Authorization",
		Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader},
	},
	{
		Field:   "password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "client_secret",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "access_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "refresh_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "id_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "assertion",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
}
