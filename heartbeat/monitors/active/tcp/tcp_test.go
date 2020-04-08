// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package tcp

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"testing"

	"github.com/elastic/go-lookslike/validator"

	"github.com/stretchr/testify/require"

	"github.com/elastic/beats/v7/heartbeat/hbtest"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	btesting "github.com/elastic/beats/v7/libbeat/testing"
	"github.com/elastic/go-lookslike"
	"github.com/elastic/go-lookslike/isdef"
	"github.com/elastic/go-lookslike/testslike"
)

func testTCPCheck(t *testing.T, host string, port uint16) *beat.Event {
	config := common.MapStr{
		"hosts":   host,
		"ports":   port,
		"timeout": "1s",
	}
	return testTCPConfigCheck(t, config, host, port)
}

func TestUpEndpointJob(t *testing.T) {
	server, port := setupServer(t, newLocalhostTestServer)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Test with domain, IPv4 and IPv6
	scenarios := []struct {
		name       string
		host       string
		isIp       bool
		expectedIp string
	}{
		{
			name:       "localhost",
			host:       "localhost",
			isIp:       false,
			expectedIp: "127.0.0.1",
		},
		{
			name:       "ipv4",
			host:       "127.0.0.1",
			isIp:       true,
			expectedIp: "127.0.0.1",
		},
		{
			name:       "ipv6",
			host:       "tcp//[::1]:80",
			isIp:       true,
			expectedIp: "::1",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			event := testTCPCheck(t, scenario.host, port)

			validators := []validator.Validator{
				hbtest.BaseChecks(serverURL.Hostname(), "up", "tcp"),
				hbtest.SummaryChecks(1, 0),
				hbtest.SimpleURLChecks(t, "tcp", scenario.host, port),
				hbtest.RespondingTCPChecks(),
			}

			if !scenario.isIp {
				validators = append(validators, hbtest.ResolveChecks(scenario.expectedIp))
			}

			testslike.Test(
				t,
				lookslike.Strict(lookslike.Compose(validators...)),
				event.Fields,
			)
		})
	}
}

func TestConnectionRefusedEndpointJob(t *testing.T) {
	ip := "127.0.0.1"
	port, err := btesting.AvailableTCP4Port()
	require.NoError(t, err)

	event := testTCPCheck(t, ip, port)

	dialErr := fmt.Sprintf("dial tcp %s:%d", ip, port)
	testslike.Test(
		t,
		lookslike.Strict(lookslike.Compose(
			hbtest.BaseChecks(ip, "down", "tcp"),
			hbtest.SummaryChecks(0, 1),
			hbtest.SimpleURLChecks(t, "tcp", ip, port),
			hbtest.ErrorChecks(dialErr, "io"),
		)),
		event.Fields,
	)
}

func TestUnreachableEndpointJob(t *testing.T) {
	ip := "203.0.113.1"
	port := uint16(1234)
	event := testTCPCheck(t, ip, port)

	dialErr := fmt.Sprintf("dial tcp %s:%d", ip, port)
	testslike.Test(
		t,
		lookslike.Strict(lookslike.Compose(
			hbtest.BaseChecks(ip, "down", "tcp"),
			hbtest.SummaryChecks(0, 1),
			hbtest.SimpleURLChecks(t, "tcp", ip, port),
			hbtest.ErrorChecks(dialErr, "io"),
		)),
		event.Fields,
	)
}

func TestCheckUp(t *testing.T) {
	host, port, ip, closeEcho, err := startEchoServer(t)
	require.NoError(t, err)
	defer closeEcho()

	configMap := common.MapStr{
		"hosts":         host,
		"ports":         port,
		"timeout":       "1s",
		"check.receive": "echo123",
		"check.send":    "echo123",
	}

	event := testTCPConfigCheck(t, configMap, host, port)

	testslike.Test(
		t,
		lookslike.Strict(lookslike.Compose(
			hbtest.BaseChecks(ip, "up", "tcp"),
			hbtest.RespondingTCPChecks(),
			hbtest.SimpleURLChecks(t, "tcp", host, port),
			hbtest.SummaryChecks(1, 0),
			hbtest.ResolveChecks(ip),
			lookslike.MustCompile(map[string]interface{}{
				"tcp": map[string]interface{}{
					"rtt.validate.us": isdef.IsDuration,
				},
			}),
		)),
		event.Fields,
	)
}

func TestCheckDown(t *testing.T) {
	host, port, ip, closeEcho, err := startEchoServer(t)
	require.NoError(t, err)
	defer closeEcho()

	configMap := common.MapStr{
		"hosts":         host,
		"ports":         port,
		"timeout":       "1s",
		"check.receive": "BOOM", // should fail
		"check.send":    "echo123",
	}

	event := testTCPConfigCheck(t, configMap, host, port)

	testslike.Test(
		t,
		lookslike.Strict(lookslike.Compose(
			hbtest.BaseChecks(ip, "down", "tcp"),
			hbtest.RespondingTCPChecks(),
			hbtest.SimpleURLChecks(t, "tcp", host, port),
			hbtest.SummaryChecks(0, 1),
			lookslike.MustCompile(map[string]interface{}{
				"resolve": map[string]interface{}{
					"ip":     ip,
					"rtt.us": isdef.IsDuration,
				},
				"tcp": map[string]interface{}{
					"rtt.validate.us": isdef.IsDuration,
				},
				"error": map[string]interface{}{
					"type":    "validate",
					"message": "received string mismatch",
				},
			}),
		)), event.Fields)
}

func TestNXDomainJob(t *testing.T) {
	host := "notadomainatallforsure.notadomain.notatldreally"
	port := uint16(1234)
	event := testTCPCheck(t, host, port)

	dialErr := fmt.Sprintf("lookup %s", host)
	testslike.Test(
		t,
		lookslike.Strict(lookslike.Compose(
			hbtest.BaseChecks("", "down", "tcp"),
			hbtest.SummaryChecks(0, 1),
			hbtest.SimpleURLChecks(t, "tcp", host, port),
			hbtest.ErrorChecks(dialErr, "io"),
		)),
		event.Fields,
	)
}

// startEchoServer starts a simple TCP echo server for testing. Only handles a single connection once.
// Note you MUST connect to this server exactly once to avoid leaking a goroutine. This is only useful
// for the specific tests used here.
func startEchoServer(t *testing.T) (host string, port uint16, ip string, close func() error, err error) {
	// Simple echo server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", 0, "", nil, err
	}
	go func() {
		conn, err := listener.Accept()
		require.NoError(t, err)
		buf := make([]byte, 1024)
		rlen, err := conn.Read(buf)
		require.NoError(t, err)
		wlen, err := conn.Write(buf[:rlen])
		require.NoError(t, err)
		// Normally we'd retry partial writes, but for tests this is OK
		require.Equal(t, wlen, rlen)
	}()

	ip, portStr, err := net.SplitHostPort(listener.Addr().String())
	portUint64, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		listener.Close()
		return "", 0, "", nil, err
	}

	return "localhost", uint16(portUint64), ip, listener.Close, nil
}
