package log

import (
	"regexp"
	"strings"
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

func NewFieldMasker(cfg MaskingRuleConfig) FieldMasker {
	fMask := FieldMasker{Field: strings.ToLower(cfg.Field), Masks: make([]Mask, 0, len(cfg.Masks))}

	for _, repCfg := range cfg.Masks {
		fMask.Masks = append(fMask.Masks, NewMask(repCfg))
	}
	for _, format := range cfg.Formats {
		switch format {
		case FieldMaskFormatHTTPHeader:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{`(?i)` + cfg.Field + `: .+?\r\n`, cfg.Field + ": ***\r\n"}))
		case FieldMaskFormatJSON:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{`(?i)"` + cfg.Field + `"\s*:\s*".*?[^\\]"`, `"` + cfg.Field + `": "***"`}))
		case FieldMaskFormatURLEncoded:
			fMask.Masks = append(fMask.Masks, NewMask(MaskConfig{`(?i)` + cfg.Field + `\s*=\s*[^&\s]+`, cfg.Field + "=***"}))
		}
	}
	return fMask
}

// Masker is used to mask various secrets in strings.
type Masker struct {
	FieldMasks []FieldMasker
}

func NewMasker(rules []MaskingRuleConfig) *Masker {
	r := &Masker{FieldMasks: make([]FieldMasker, 0, len(rules))}
	for _, rule := range rules {
		r.FieldMasks = append(r.FieldMasks, NewFieldMasker(rule))
	}
	return r
}

func (r *Masker) Mask(s string) string {
	lower := strings.ToLower(s)
	for _, fieldMask := range r.FieldMasks {
		if strings.Contains(lower, fieldMask.Field) {
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
