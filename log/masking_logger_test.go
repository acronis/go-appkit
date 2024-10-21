package log_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ssgreg/logf"
	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
)

func TestMaskingLogger(t *testing.T) {
	recorder := logtest.NewRecorder()
	maskingLog := log.NewMaskingLogger(recorder, log.NewMasker(log.DefaultMasks))

	checkRecordedLogAndReset := func(wantText string, wantLevel log.Level, wantFields ...log.Field) {
		entries := recorder.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, wantText, entries[0].Text)
		require.Equal(t, wantLevel, entries[0].Level)
		require.ElementsMatch(t, wantFields, entries[0].Fields)
		recorder.Reset()
	}

	maskingLog.Error("client_secret=123", log.String("value", "client_secret=333"), log.Error(errors.New("client_secret=665")))
	checkRecordedLogAndReset("client_secret=***", log.LevelError, log.String("value", "client_secret=***"),
		log.Error(errors.New("client_secret=***")))

	maskingLog.Info("client_secret=123", log.String("value", "client_secret=346"), log.Error(errors.New("client_secret=668")))
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, log.String("value", "client_secret=***"),
		log.Error(errors.New("client_secret=***")))

	maskingLog.Warn("client_secret=123", log.String("value", "client_secret=332"), log.Error(errors.New("client_secret=965")))
	checkRecordedLogAndReset("client_secret=***", log.LevelWarn, log.String("value", "client_secret=***"),
		log.Error(errors.New("client_secret=***")))

	maskingLog.Debug("client_secret=123", log.String("value", "client_secret=333"), log.Error(errors.New("client_secret=665")))
	checkRecordedLogAndReset("client_secret=***", log.LevelDebug, log.String("value", "client_secret=***"),
		log.Error(errors.New("client_secret=***")))

	maskingLog.Errorf("client_secret=%d", 123)
	checkRecordedLogAndReset("client_secret=***", log.LevelError)

	maskingLog.Infof("client_secret=%d", 123)
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo)

	maskingLog.Warnf("client_secret=%d", 123)
	checkRecordedLogAndReset("client_secret=***", log.LevelWarn)

	maskingLog.Debugf("client_secret=%d", 123)
	checkRecordedLogAndReset("client_secret=***", log.LevelDebug)

	maskingLog.With(log.String("value", "client_secret=346"), log.NamedError("error_field", errors.New("client_secret=668"))).Info("client_secret=123")
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, log.String("value", "client_secret=***"),
		log.NamedError("error_field", errors.New("client_secret=***")))

	maskingLog.AtLevel(log.LevelInfo, func(l log.LogFunc) {
		l("client_secret=123", log.String("value", "client_secret=123"))
	})
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, log.String("value", "client_secret=***"))

	maskingLog.WithLevel(log.LevelInfo).Info("client_secret=123", log.String("value", "client_secret=***"))
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, log.String("value", "client_secret=***"))

	maskingLog.Error("abc", log.Error(fmtError{errors.New("client_secret=665")}))
	errS := fmt.Sprintf("%s", recorder.Entries()[0].Fields[0].Any)
	require.Contains(t, errS, "client_secret=***")
	require.Contains(t, errS, "password=***")
	recorder.Reset()

	maskingLog.Info("client_secret=123", log.Strings("value", []string{"client_secret=346"}))
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, log.Strings("value", []string{"client_secret=***"}))

	maskingLog.Info("client_secret=123", log.Bytes("value", []byte("client_secret=346")))
	checkRecordedLogAndReset("client_secret=***", log.LevelInfo, logf.ConstBytes("value", []byte("client_secret=***")))
}

type fmtError struct {
	err error
}

func (e fmtError) Error() string {
	return e.err.Error()
}

func (e fmtError) Format(f fmt.State, verb rune) {
	_, _ = io.WriteString(f, e.Error()+" password=123")
}

var logFile = "output.log"

func BenchmarkMaskingLogger(b *testing.B) {
	defaultLogger, closer := log.NewLogger(&log.Config{
		Output: log.OutputFile, Format: log.FormatJSON, Level: log.LevelInfo, ErrorVerboseSuffix: "_verbose", AddCaller: true, File: log.FileOutputConfig{
			Path: logFile,
			Rotation: log.FileRotationConfig{
				MaxSize: 2 << 30,
			},
		},
	})
	defer func() {
		closer()
		_ = os.Remove(logFile)
	}()

	for _, test := range []struct {
		name   string
		logger log.FieldLogger
	}{
		{
			name:   "Logger (file)",
			logger: defaultLogger,
		},
		{
			name:   "MaskingLogger",
			logger: log.NewMaskingLogger(defaultLogger, log.NewMasker(log.DefaultMasks)),
		},
	} {
		b.Run(test.name, func(b *testing.B) {
			loggingOpts := middleware.LoggingOpts{
				RequestStart:           true,
				AddRequestInfoToLogger: true,
				RequestHeaders:         map[string]string{"X-Original-URI": "original_uri", "Referer": "referer"},
			}
			logger := test.logger.With(
				log.String("logger", "ApiGateway"),
				log.String("build", "1257"),
				log.String("source", "api-gateway"),
			)
			mw := middleware.LoggingWithOpts(logger, loggingOpts)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middleware.GetLoggerFromContext(r.Context()).Info("custom message")
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, "https://au1-cloud.acronis.com/api/gateway/authorize", nil)
				req.Header.Set("X-Forwarded-For", "181.58.39.81")
				req.Header.Set("X-Original-URI", "/bc/api/task_manager/v2/activities")
				req.Header.Set("Referer", "https://au1-cloud.acronis.com/ui/")
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")

				ctx := req.Context()
				ctx = middleware.NewContextWithRequestID(ctx, "03497b44a93143e2c5ff8e0e0e57232a")
				ctx = middleware.NewContextWithInternalRequestID(ctx, "cs4kq8gcedg46frf7v8g")
				req = req.WithContext(ctx)
				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)
			}
		})
	}
}
