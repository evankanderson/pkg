/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestHTTPRoundTripper(t *testing.T) {
	wants := sets.NewString()
	frt := func(key string) http.RoundTripper {
		return RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			wants.Insert(key)
			return nil, nil
		})
	}

	rt := newAutoTransport(frt("v1"), frt("v2"))

	examples := []struct {
		label      string
		protoMajor int
		want       string
	}{{
		label:      "use default transport for HTTP1",
		protoMajor: 1,
		want:       "v1",
	}, {
		label:      "use h2c transport for HTTP2",
		protoMajor: 2,
		want:       "v2",
	}, {
		label:      "use default transport for all others",
		protoMajor: 99,
		want:       "v1",
	}}

	for _, e := range examples {
		t.Run(e.label, func(t *testing.T) {
			wants.Delete(e.want)
			r := &http.Request{ProtoMajor: e.protoMajor}
			rt.RoundTrip(r)

			if !wants.Has(e.want) {
				t.Error("Wrong transport selected for request.")
			}
		})
	}
}

//goland:noinspection HttpUrlsUsage
const HTTPPrefix = "http://"

func TestDialWithBackoff(t *testing.T) {
	t.Parallel()
	t.Run("ConnectionRefused", testDialWithBackoffConnectionRefused)
	t.Run("Timeout", testDialWithBackoffTimeout)
	t.Run("Success", testDialWithBackoffSuccess)
}

func testDialWithBackoffConnectionRefused(t *testing.T) {
	port := findUnusedPortOrFail(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	c, err := DialWithBackOff(context.Background(), "tcp4", addr)
	closeOrFail(t, c)
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("Unexpected error: %+v", err)
	}
}

func testDialWithBackoffTimeout(t *testing.T) {
	// Make the test short.
	bo := backOffTemplate
	bo.Steps = 1
	bo.Duration = 1 * time.Millisecond

	// Golang doesn't allow us to set the backlog argument on syscall.Listen ourselves.
	closer, addr, err := listenOne()
	if err != nil {
		t.Fatalf("Unable to listen on %s: %s", addr, err)
	}
	defer closer()
	// Occupy the single listen backlog for the entire test.
	c1, err := dialBackOffHelper(context.Background(), "tcp4", addr.String(), bo, bo.Duration)
	if err != nil {
		t.Fatal("Unable to occupy single backlog connection: ", err)
	}
	defer c1.Close()
	// Since the backlog is full, the next request should time out.

	// Timeout. Use non-routable IP. See: https://stackoverflow.com/a/31581323/844449
	c, err := dialBackOffHelper(
		context.Background(), "tcp4", addr.String() /*"10.0.0.0:81"*/, bo, bo.Duration)
	if err == nil {
		closeOrFail(t, c)
		t.Error("Unexpected success dialing")
	}
	if !errors.Is(err, ErrTimeoutDialing) {
		t.Errorf("Unexpected error: %+v", err)
	}
}

func testDialWithBackoffSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer s.Close()
	addr := strings.TrimPrefix(s.URL, HTTPPrefix)

	c, err := DialWithBackOff(context.Background(), "tcp4", addr)
	if err != nil {
		t.Fatal("Dial error =", err)
	}
	closeOrFail(t, c)
}

func closeOrFail(tb testing.TB, con io.Closer) {
	tb.Helper()
	if con == nil {
		return
	}
	if err := con.Close(); err != nil {
		tb.Fatal(err)
	}
}

func findUnusedPortOrFail(tb testing.TB) int {
	tb.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		tb.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}
	defer closeOrFail(tb, l)
	return l.Addr().(*net.TCPAddr).Port
}

// Golang doesn't allow us to set the backlog argument on syscall.Listen from
// net.ListenTCP, so we need to get directly into syscall land.
func listenOne() (func(), *net.TCPAddr, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("Couldn't get socket: %w", err)
	}
	sa := &syscall.SockaddrInet4{
		Port: 0,
		Addr: [4]byte{127, 0, 0, 1},
	}
	if err = syscall.Bind(fd, sa); err != nil {
		return nil, nil, fmt.Errorf("Unable to bind: %w", err)
	}
	if err = syscall.Listen(fd, 0); err != nil {
		return nil, nil, fmt.Errorf("Unable to Listen: %w", err)
	}
	closer := func() { syscall.Close(fd) }
	listenaddr, err := syscall.Getsockname(fd)
	if err != nil {
		closer()
		return nil, nil, fmt.Errorf("Could not get sockname: %w", err)
	}
	sa = listenaddr.(*syscall.SockaddrInet4)
	addr := &net.TCPAddr{
		IP:   sa.Addr[:],
		Port: sa.Port,
	}
	return closer, addr, nil
}
