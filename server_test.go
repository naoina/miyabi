package miyabi_test

import (
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/naoina/miyabi"
)

const (
	addr = "127.0.0.1:0"
)

func newTestListener(t *testing.T) net.Listener {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestServer_Serve(t *testing.T) {
	done := make(chan struct{}, 1)
	server := &miyabi.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- struct{}{}
	})}
	l := newTestListener(t)
	defer l.Close()
	go func() {
		if err := server.Serve(l); err != nil {
			t.Errorf("server.Serve(l) => %#v; want nil", err)
		}
		done <- struct{}{}
	}()
	_, err := http.Get("http://" + l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("timeout")
	}

	l.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("timeout")
	}
}

func TestServer_Serve_gracefulShutdownDefaultSignal(t *testing.T) {
	testServerServeGracefulShutdown(t)
}

func TestServer_Serve_gracefulShutdownAnotherSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGQUIT} {
		origSignal := miyabi.ShutdownSignal
		miyabi.ShutdownSignal = sig
		defer func() {
			miyabi.ShutdownSignal = origSignal
		}()
		testServerServeGracefulShutdown(t)
	}
}

func testServerServeGracefulShutdown(t *testing.T) {
	done := make(chan struct{})
	server := &miyabi.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- struct{}{}
	})}
	l := newTestListener(t)
	defer l.Close()
	go server.Serve(l)
	wait := make(chan struct{})
	go func() {
		if _, err := http.Get("http://" + l.Addr().String()); err != nil {
			t.Errorf("http.Get => %v; want nil", err)
		}
		wait <- struct{}{}
	}()
	<-time.After(1 * time.Second)
	pid := os.Getpid()
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(miyabi.ShutdownSignal); err != nil {
		t.Fatal(err)
	}
	go func() {
		<-time.After(1 * time.Second)
		if _, err = http.Get("http://" + l.Addr().String()); err == nil {
			t.Errorf("http.Get after shutdown => nil; want error")
		}
		<-done
		<-wait
		wait <- struct{}{}
	}()
	select {
	case <-wait:
	case <-time.After(5 * time.Second):
		t.Errorf("timeout")
	}
}

func TestServerState_StateStart(t *testing.T) {
	done := make(chan struct{})
	origServerState := miyabi.ServerState
	miyabi.ServerState = func(state miyabi.State) {
		switch state {
		case miyabi.StateStart:
			done <- struct{}{}
		}
	}
	defer func() {
		miyabi.ServerState = origServerState
	}()
	go miyabi.ListenAndServe(addr, nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("timeout")
	}
}

func TestServerState_StateShutdown(t *testing.T) {
	done := make(chan struct{})
	started := make(chan struct{})
	origServerState := miyabi.ServerState
	miyabi.ServerState = func(state miyabi.State) {
		switch state {
		case miyabi.StateStart:
			started <- struct{}{}
		case miyabi.StateShutdown:
			done <- struct{}{}
		}
	}
	defer func() {
		miyabi.ServerState = origServerState
	}()
	go miyabi.ListenAndServe(addr, nil)
	select {
	case <-started:
		pid := os.Getpid()
		p, err := os.FindProcess(pid)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Signal(miyabi.ShutdownSignal); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("timeout")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("timeout")
	}
}
