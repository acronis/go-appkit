// nolint: lll
package log

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMasker(t *testing.T) {
	replAToB := MaskingRuleConfig{Masks: []MaskConfig{{`A`, `B`}}}
	replBToA := MaskingRuleConfig{Masks: []MaskConfig{{`B`, `A`}}}
	cases := []struct {
		ruleConfig []MaskingRuleConfig
		input      string
		expected   string
	}{
		{
			[]MaskingRuleConfig{replAToB},
			"ABA",
			"BBB",
		},
		{
			[]MaskingRuleConfig{replAToB, replBToA},
			"ABA",
			"AAA",
		},
		{
			[]MaskingRuleConfig{replBToA, replAToB},
			"ABA",
			"BBB",
		},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			m := NewMasker(c.ruleConfig)
			out := m.Mask(c.input)
			require.Equal(t, c.expected, out)
		})
	}
}

func TestDefaultMasks(t *testing.T) {
	tests := []struct {
		name, s, expected string
	}{
		{
			name:     "simple",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
		{
			name:     "short",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***",
		},
		{
			name:     "after",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&client_secret=***",
		},
		{
			name:     "middle",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
		{
			name:     "new line",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000\n",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000\n",
		},
		{
			name:     "new line 2",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg\n&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***\n&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
		{
			name:     "crlf",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000\r\n",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000\r\n",
		},
		{
			name:     "crlf2",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg\r\n&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***\r\n&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
		{
			name:     "Authorization",
			s:        "GET /abc HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer abcdef\r\nContent-Length: 3691\r\n\r\n",
			expected: "GET /abc HTTP/1.1\r\nHost: example.com\r\nAuthorization: ***\r\nContent-Length: 3691\r\n\r\n",
		},
		{
			name:     "authorization",
			s:        "GET /abc HTTP/1.1\r\nHost: example.com\r\nauthorization: Bearer abcdef\r\nContent-Length: 3691\r\n\r\n",
			expected: "GET /abc HTTP/1.1\r\nHost: example.com\r\nAuthorization: ***\r\nContent-Length: 3691\r\n\r\n",
		},
		{
			name:     "password JSON",
			s:        `{"password": "abc"},`,
			expected: `{"password": "***"},`,
		},
		{
			name:     "password URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&password=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&password=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "client_secret JSON",
			s:        `{"client_secret": "abc"},`,
			expected: `{"client_secret": "***"},`,
		},
		{
			name:     "client_secret URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "access_token JSON",
			s:        `{"access_token": "abc"},`,
			expected: `{"access_token": "***"},`,
		},
		{
			name:     "access_token URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&access_token=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&access_token=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "refresh_token JSON",
			s:        `{"refresh_token": "abc"},`,
			expected: `{"refresh_token": "***"},`,
		},
		{
			name:     "refresh_token URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&refresh_token=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&refresh_token=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "id_token JSON",
			s:        `{"id_token": "ab\"c"},`,
			expected: `{"id_token": "***"},`,
		},
		{
			name:     "id_token URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&id_token=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&id_token=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "assertion JSON",
			s:        `{"assertion": "abc"},`,
			expected: `{"assertion": "***"},`,
		},
		{
			name:     "assertion URL encoded",
			s:        `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=asdf$%^*(&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
			expected: `grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000`,
		},
		{
			name:     "Handle multiple masks",
			s:        `POST / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test/1/\r\nContent-Length: 123\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_id=f1e3bb97-552d-4a21-aa7d-543ad8bde840&client_secret=supersecret&refresh_token=token123&id_token=idToken`,
			expected: `POST / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test/1/\r\nContent-Length: 123\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_id=f1e3bb97-552d-4a21-aa7d-543ad8bde840&client_secret=***&refresh_token=***&id_token=***`,
		},
		{
			name:     "No masking needed",
			s:        `POST / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test/1/\r\nContent-Length: 123\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_id=f1e3bb97-552d-4a21-aa7d-543ad8bde840&grant_type=test`,
			expected: `POST / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test/1/\r\nContent-Length: 123\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_id=f1e3bb97-552d-4a21-aa7d-543ad8bde840&grant_type=test`,
		},
	}

	masker := NewMasker(DefaultMasks)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := masker.Mask(test.s)
			require.Equal(t, test.expected, out)
		})
	}
}

var testMMasks = []MaskingRuleConfig{
	{
		Field:   "Authorization",
		Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader},
	},
	{
		Field:   "authorization",
		Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader},
	},
	{
		Field:   "Password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ClientSecret",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "client_secret",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "AccessToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "access_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "RefreshToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "refresh_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "IdToken",
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
	{
		Field:   "Pwd",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "pwd",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Salt",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "salt",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Tenant",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "tenant",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Cookie",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "cookie",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ApiKey",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "api_key",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CreditCard",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "credit_card",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "SSN",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ssn",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "SocialSecurityNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "social_security_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "SessionID",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "session_id",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Secret",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "secret",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "AuthToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "auth_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "AccessKey",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "access_key",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "PrivateKey",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "private_key",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "EncryptionKey",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "encryption_key",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "DatabasePassword",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "database_password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "DbPassword",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "db_password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "FtpPassword",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ftp_password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "SSHKey",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ssh_key",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "OAuthToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "oauth_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BearerToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "bearer_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "JWT",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "jwt",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "VerificationCode",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "verification_code",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "OneTimePassword",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "one_time_password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "OTP",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BearerToken",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "bearer_token",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "JWT",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "jwt",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "VerificationCode",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "verification_code",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "OneTimePassword",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "one_time_password",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "OTP",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "otp",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	// --- Дополнительные маски ---
	{
		Field:   "Email",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "email",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "EmailAddress",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "email_address",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "PhoneNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "phone_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Phone",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "phone",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CreditCardNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "credit_card_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CVV",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "cvv",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CVC",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "cvc",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "ExpirationDate",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "expiration_date",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CardNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "card_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BankAccount",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "bank_account",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BankAccountNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "bank_account_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BankRoutingNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "bank_routing_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "CreditScore",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "credit_score",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "HealthInsuranceNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "health_insurance_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "DriversLicenseNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "drivers_license_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "PassportNumber",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "passport_number",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "TaxID",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "tax_id",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "PII",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "pii",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "PIIData",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "pii_data",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "FinancialInfo",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "financial_info",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "MedicalRecord",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "medical_record",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "BiometricData",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "biometric_data",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "LocationData",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "location_data",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "DeviceID",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "device_id",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "MacAddress",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "mac_address",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "Geolocation",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
	{
		Field:   "geolocation",
		Formats: []FieldMaskFormat{FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
	},
}

func TestCustomLargeAmountOfMasks(t *testing.T) {
	tests := []struct {
		name, s, expected string
	}{
		{
			name:     "simple",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
		{
			name:     "short",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***",
		},
		{
			name:     "after",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&client_secret=***",
		},
		{
			name:     "middle",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&VerificationCode=ABCD&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&VerificationCode=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
	}

	masker := NewMasker(testMMasks)
	for _, test := range tests {
		subtest := test
		t.Run(subtest.name, func(t *testing.T) {
			// Enable parallel execution to check races
			t.Parallel()

			out := masker.Mask(subtest.s)
			require.Equal(t, subtest.expected, out)
		})
	}
}

func TestHybridMasker(t *testing.T) {
	tests := []struct {
		name, s, expected string
	}{
		{
			name:     "simple",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&AAAAA",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&*****",
		},
		{
			name:     "middle",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nAAAAA\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\n*****\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
	}

	masksForTest := append(testMMasks, MaskingRuleConfig{Masks: []MaskConfig{{`AAAAA`, `*****`}}})

	masker := NewMasker(masksForTest)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := masker.Mask(test.s)
			require.Equal(t, test.expected, out)
		})
	}
}

func TestHybridMaskerWithShuffle(t *testing.T) {
	tests := []struct {
		name, s, expected string
	}{
		{
			name:     "simple",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nBBBBBUser-Agent: test-agent\r\nContent-Length: 3691\r\nDDDDDContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&AAAAA",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\n*****User-Agent: test-agent\r\nContent-Length: 3691\r\n*****Content-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\nclient_secret=***&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000&*****",
		},
		{
			name:     "middle",
			s:        "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\nAAAAA\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=eyJhbGciOiJSUzI1NiIsImVhcCI6MSwiaXJpIjoiY2hlNWphMmowaW9kN3E0c21kbDAiLCJraWQiOiU1NzVkYjAifQ.eyJhdWQiOiJ1cy1jbG91ZC5hY3JvbmlzLmNvbSIs7QI0ctcs7ZN8OsCDUxhM4liWPGg&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
			expected: "POST /idp/token HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test-agent\r\n*****\r\nContent-Length: 3691\r\nContent-Type: application/x-www-form-urlencoded\r\nAccept-Encoding: gzip\r\n\r\ngrant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&client_secret=***&scope=scope1%3Atenant-id%0000000000-0000-0000-0000-000000000000",
		},
	}

	// shuffle
	masksForTest := append(testMMasks,
		MaskingRuleConfig{Masks: []MaskConfig{{`AAAAA`, `*****`}}},
		MaskingRuleConfig{Masks: []MaskConfig{{`BBBBB`, `*****`}}},
		MaskingRuleConfig{Masks: []MaskConfig{{`CCCCC`, `*****`}}},
		MaskingRuleConfig{Masks: []MaskConfig{{`DDDDD`, `*****`}}},
	)

	masker := NewMasker(masksForTest)
	for _, test := range tests {
		for i := 0; i < 10; i++ {
			rand.Shuffle(len(masksForTest), func(i, j int) {
				masksForTest[i], masksForTest[j] = masksForTest[j], masksForTest[i]
			})
			t.Run(test.name, func(t *testing.T) {
				out := masker.Mask(test.s)
				require.Equal(t, test.expected, out)
			})
		}
	}
}

func BenchmarkMasker(b *testing.B) {
	r := NewMasker(testMMasks)
	b.ResetTimer()
	for _, test := range []struct{ name, text string }{
		{
			name: "0 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&api_kSDFey=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "1 match",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "3 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refresh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&&pwWd=GGGG&sOlt=HHHH&tenant=123123123`,
		},
	} {
		b.Run(test.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r.Mask(test.text)
			}
		})
	}
}

func BenchmarkMaskerWithContains(b *testing.B) {
	r := NewMasker(testMMasks)

	maskContains := func(r *Masker, s string) string {
		lower := strings.ToLower(s)
		for _, fieldMask := range r.fieldMasks {
			if strings.Contains(lower, fieldMask.Field) {
				for _, rep := range fieldMask.Masks {
					s = rep.RegExp.ReplaceAllString(s, rep.Mask)
				}
			}
		}
		return s
	}

	b.ResetTimer()
	for _, test := range []struct{ name, text string }{
		{
			name: "0 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&api_kSDFey=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "1 match",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "3 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refresh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&&pwWd=GGGG&sOlt=HHHH&tenant=123123123`,
		},
	} {
		b.Run(test.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				maskContains(r, test.text)
			}
		})
	}
}

func BenchmarkParallelMasker(b *testing.B) {
	r := NewMasker(testMMasks)
	b.ResetTimer()
	for _, test := range []struct{ name, text string }{
		{
			name: "0 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&api_kSDFey=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "1 match",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&pwWd=GGGG&sOlt=HHHH&tenNant=123123123`,
		},
		{
			name: "3 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refresh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC&&pwWd=GGGG&sOlt=HHHH&tenant=123123123`,
		},
	} {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				for i := 0; i < b.N; i++ {
					r.Mask(test.text)
				}
			}
		})
	}
}
