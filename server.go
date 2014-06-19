// Package miyabi provides graceful version of net/http compatible HTTP server.
package miyabi

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	// ShutdownSignal is signal for graceful shutdown.
	// syscall.SIGTERM by default. Please set another signal if you want.
	ShutdownSignal = syscall.SIGTERM
)

// ListenAndServe acts like http.ListenAndServe but can be graceful shutdown.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &Server{Addr: addr, Handler: handler}
	return server.ListenAndServe()
}

// ListenAndServeTLS acts like http.ListenAndServeTLS but can be graceful shutdown.
func ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	server := &Server{Addr: addr, Handler: handler}
	return server.ListenAndServeTLS(certFile, keyFile)
}

// Server is similar to http.Server.
// However, ListenAndServe, ListenAndServeTLS and Serve can be graceful shutdown.
type Server http.Server

// ListenAndServe acts like http.Server.ListenAndServe but can be graceful shutdown.
func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}

// ListenAndServeTLS acts like http.Server.ListenAndServeTLS but can be graceful shutdown.
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}
	config := &tls.Config{}
	if srv.TLSConfig != nil {
		*config = *srv.TLSConfig
	}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}
	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	tlsListener := tls.NewListener(tcpKeepAliveListener{ln.(*net.TCPListener)}, config)
	return srv.Serve(tlsListener)
}

// Serve acts like http.Server.Serve but can be graceful shutdown.
func (srv *Server) Serve(l net.Listener) error {
	conns := make(map[net.Conn]struct{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	srv.ConnState = func(conn net.Conn, state http.ConnState) {
		mu.Lock()
		switch state {
		case http.StateActive:
			conns[conn] = struct{}{}
			wg.Add(1)
		case http.StateIdle, http.StateClosed:
			if _, exists := conns[conn]; exists {
				delete(conns, conn)
				wg.Done()
			}
		}
		mu.Unlock()
	}
	srv.startWaitSignals(l)
	err := (*http.Server)(srv).Serve(l)
	wg.Wait()
	return err
}

// SetKeepAlivesEnabled is same as http.Server.SetKeepAlivesEnabled.
func (srv *Server) SetKeepAlivesEnabled(v bool) {
	(*http.Server)(srv).SetKeepAlivesEnabled(v)
}

func (srv *Server) startWaitSignals(l net.Listener) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, ShutdownSignal)
	go func() {
		sig := <-c
		srv.SetKeepAlivesEnabled(false)
		switch sig {
		case syscall.SIGINT, ShutdownSignal:
			l.Close()
		}
	}()
}

// tcpKeepAliveListener is copy from net/http.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

// Accept is copy from net/http.
func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
