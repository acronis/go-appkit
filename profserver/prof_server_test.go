/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package profserver

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/log/logtest"
	"git.acronis.com/abc/go-libs/v2/testutil"
)

func TestProfServer_Start(t *testing.T) {
	addr := testutil.GetLocalAddrWithFreeTCPPort()

	profServer := New(&Config{Address: addr}, logtest.NewRecorder())
	fatalErr := make(chan error, 1)
	go profServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))
	defer func() {
		require.NoError(t, profServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	resp, err := http.Get(profServer.URL + "/debug/pprof/")
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, len(respBody) > 0)
}
