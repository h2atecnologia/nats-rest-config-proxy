// Copyright 2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"
)

var testPort int = 4567

func newTestServer() (*Server, error) {
	dir, err := ioutil.TempDir("", "acl-proxy-data-dir-")
	if err != nil {
		return nil, err
	}
	opts := &Options{
		NoSignals: true,
		NoLog:     true,
		Debug:     true,
		Trace:     true,
		Host:      "localhost",
		Port:      testPort,
		DataDir:   dir,
	}
	if os.Getenv("DEBUG") == "true" {
		opts.NoLog = false
	}
	testPort += 1
	s := &Server{opts: opts}

	// Setup test server for handler without binding port
	l := NewLogger(s.opts)
	l.logger.SetOutput(ioutil.Discard)
	s.log = l
	err = s.setupStoreDirectories()
	if err != nil {
		return nil, err
	}
	return s, nil

}

func waitServerIsReady(t *testing.T, ctx context.Context, s *Server) {
	host := s.opts.Host
	port := s.opts.Port
	for range time.NewTicker(50 * time.Millisecond).C {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				t.Fatal(ctx.Err())
			}
		default:
		}

		resp, err := http.Get(fmt.Sprintf("http://%s:%d/healthz", host, port))
		if err != nil {
			t.Logf("Error: %s", err)
			continue
		}
		if resp != nil && resp.StatusCode == 200 {
			break
		}
	}
}

func waitServerIsDone(t *testing.T, ctx context.Context, s *Server) {
	host := s.opts.Host
	port := s.opts.Port
	for range time.NewTicker(50 * time.Millisecond).C {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				t.Fatal(ctx.Err())
			}
		default:
		}

		resp, err := http.Get(fmt.Sprintf("http://%s:%d/healthz", host, port))
		if err == nil && resp.StatusCode != 200 {
			continue
		}
		break
	}
}

func curl(method string, endpoint string, payload []byte) (*http.Response, []byte, error) {
	result, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, err
	}
	e := fmt.Sprintf("%s://%s%s", result.Scheme, result.Host, result.Path)
	buf := bytes.NewBuffer([]byte(payload))
	req, err := http.NewRequest(method, e, buf)
	if err != nil {
		return nil, nil, err
	}
	if len(result.Query()) > 0 {
		q := req.URL.Query()
		for k, v := range result.Query() {
			q.Add(k, string(v[0]))
		}
		req.URL.RawQuery = q.Encode()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}

func TestServerSetup(t *testing.T) {
	s, err := newTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(s.opts.DataDir)
	err = s.setupStoreDirectories()
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(s.resourcesDir())
	if err != nil {
		t.Error(err)
	}
	_, err = os.Stat(s.snapshotsDir())
	if err != nil {
		t.Error(err)
	}
	_, err = os.Stat(s.currentConfigDir())
	if err != nil {
		t.Error(err)
	}
}

func TestNewServer(t *testing.T) {
	expected := &Server{opts: &Options{}}
	got := NewServer(nil)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected %+v, got: %+v", expected, got)
	}
}

func TestServerSignalHandler(t *testing.T) {
	s, err := newTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(s.opts.DataDir)
	s.opts.NoSignals = false

	called := make(chan struct{}, 0)
	s.quit = func() {
		called <- struct{}{}
	}
	ctx, done := context.WithCancel(context.Background())
	go s.SetupSignalHandler(ctx)

	time.Sleep(100 * time.Millisecond)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)

	select {
	case <-time.After(1 * time.Second):
		t.Fatal("Time out waiting for server to exit")
	case <-called:
		done()
	}
}
