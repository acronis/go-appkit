// nolint: lll
package log

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMasker(t *testing.T) {
	replAToB := MaskingRuleConfig{Masks: []MaskConfig{{`A`, `B`}}}
	replBToA := MaskingRuleConfig{Masks: []MaskConfig{{`B`, `A`}}}
	cases := []struct {
		masker   *Masker
		input    string
		expected string
	}{
		{
			NewMasker([]MaskingRuleConfig{replAToB}),
			"ABA",
			"BBB",
		},
		{
			NewMasker([]MaskingRuleConfig{replAToB, replBToA}),
			"ABA",
			"AAA",
		},
		{
			NewMasker([]MaskingRuleConfig{replBToA, replAToB}),
			"ABA",
			"BBB",
		},
	}
	for _, c := range cases {
		out := c.masker.Mask(c.input)
		require.Equal(t, c.expected, out)
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

func BenchmarkMasker(b *testing.B) {
	r := NewMasker(DefaultMasks)

	for _, test := range []struct{ name, text string }{
		{
			name: "0 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&api_kSDFey=lskdjflksjdl& AuthorSDFization: Bearer ABC`,
		},
		{
			name: "1 match",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refreSDFsh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC`,
		},
		{
			name: "2 matches",
			text: `{"passwSDFord": "abc", "clientSDF_secret": "clientkey123", "accesSDFSs_token": "accessToken123", "refreshSDF_token": "refresh123"}, assertSDFion=abcdef&client_sSDFecret=sjdlkfjl&refresh_token=sjdkjlk&id_token=lskdjflksjdl& AuthorSDFization: Bearer ABC`,
		},
	} {
		b.Run(test.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r.Mask(test.text)
			}
		})
	}
}
