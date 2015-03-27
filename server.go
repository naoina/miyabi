// Package miyabi provides graceful version of net/http compatible HTTP server.
package miyabi

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

var (
	// ShutdownSignal is the signal for graceful shutdown.
	// syscall.SIGTERM by default. Please set another signal if you want.
	ShutdownSignal = syscall.SIGTERM

	// RestartSignal is the signal for graceful restart.
	// syscall.SIGHUP by default. Please set another signal if you want.
	RestartSignal = syscall.SIGHUP

	// Timeout specifies the timeout for terminate of the old process.
	// A zero value disables the timeout.
	Timeout = 3 * time.Minute

	// ServerState specifies the optional callback function that is called
	// when the server changes state. See the State type and associated
	// constants for details.
	ServerState func(state State)

	// FDEnvKey is the environment variable name of inherited file descriptor for graceful restart.
	FDEnvKey = "MIYABI_FD"

	errNotForked = errors.New("server isn't forked")
)

// ListenAndServe acts like http.ListenAndServe but can be graceful shutdown
// and restart.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &Server{Addr: addr, Handler: handler}
	return server.ListenAndServe()
}

// ListenAndServeTLS acts like http.ListenAndServeTLS but can be graceful
// shutdown and restart.
func ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	server := &Server{Addr: addr, Handler: handler}
	return server.ListenAndServeTLS(certFile, keyFile)
}

// Server is similar to http.Server.
// However, ListenAndServe, ListenAndServeTLS and Serve can be graceful
// shutdown and restart.
type Server http.Server

// ListenAndServe acts like http.Server.ListenAndServe but can be graceful
// shutdown and restart.
func (srv *Server) ListenAndServe() error {
	if srv.isMaster() {
		l, err := srv.listen()
		if err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			return srv.Serve(tcpKeepAliveListener{l.(*net.TCPListener)})
		}
		return srv.supervise(l)
	}
	ln, err := srv.listenerFromFDEnv()
	if err != nil {
		return err
	}
	return srv.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}

// ListenAndServeTLS acts like http.Server.ListenAndServeTLS but can be
// graceful shutdown and restart.
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	if srv.isMaster() {
		l, err := srv.listenTLS(certFile, keyFile)
		if err != nil {
			return err
		}
		return srv.supervise(l)
	}
	ln, err := srv.listenerFromFDEnv()
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

// Serve acts like http.Server.Serve but can be graceful shutdown.
// If you want to graceful restart, use ListenAndServe or ListenAndServeTLS instead.
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
	if err, ok := err.(*net.OpError); ok {
		op := err.Op
		if runtime.GOOS == "windows" && op == "AcceptEx" {
			op = "accept"
		}
		if op == "accept" && err.Err.Error() == "use of closed network connection" {
			return nil
		}
	}
	return err
}

// SetKeepAlivesEnabled is same as http.Server.SetKeepAlivesEnabled.
func (srv *Server) SetKeepAlivesEnabled(v bool) {
	(*http.Server)(srv).SetKeepAlivesEnabled(v)
}

type listener interface {
	net.Listener

	File() (*os.File, error)
}

func (srv *Server) listen() (listener, error) {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return l.(listener), nil
}

func (srv *Server) listenTLS(certFile, keyFile string) (listener, error) {
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
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	tlsListener := tls.NewListener(tcpKeepAliveListener{ln.(*net.TCPListener)}, config)
	return tlsListener.(listener), nil
}

func (srv *Server) startWaitSignals(l net.Listener) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, ShutdownSignal)
	go func() {
		sig := <-c
		srv.SetKeepAlivesEnabled(false)
		switch sig {
		case syscall.SIGINT, ShutdownSignal:
			signal.Stop(c)
			l.Close()
		}
	}()
}

// isMaster returns whether the current process is master.
func (srv *Server) isMaster() bool {
	return os.Getenv(FDEnvKey) == ""
}

func (srv *Server) supervise(l listener) error {
	p, err := srv.forkExec(l)
	if err != nil {
		return err
	}
	if ServerState != nil {
		ServerState(StateStart)
	}
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, ShutdownSignal, RestartSignal)
	for {
		switch sig := <-c; sig {
		case RestartSignal:
			child, err := srv.forkExec(l)
			if err != nil {
				return err
			}
			p.Signal(ShutdownSignal)
			timer := time.AfterFunc(Timeout, func() {
				p.Kill()
			})
			p.Wait()
			timer.Stop()
			p = child
			if ServerState != nil {
				ServerState(StateRestart)
			}
		case syscall.SIGINT, ShutdownSignal:
			signal.Stop(c)
			l.Close()
			p.Signal(ShutdownSignal)
			timer := time.AfterFunc(Timeout, func() {
				p.Kill()
			})
			_, err := p.Wait()
			timer.Stop()
			if ServerState != nil {
				ServerState(StateShutdown)
			}
			return err
		}
	}
}

func (srv *Server) listenerFromFDEnv() (net.Listener, error) {
	fd, err := srv.getFD()
	if err != nil {
		return nil, err
	}
	file := os.NewFile(fd, "listen socket")
	defer file.Close()
	return net.FileListener(file)
}

// getFD gets file descriptor of listen socket from environment variable.
func (srv *Server) getFD() (uintptr, error) {
	fdStr := os.Getenv(FDEnvKey)
	if fdStr == "" {
		return 0, errNotForked
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil {
		return 0, err
	}
	return uintptr(fd), nil
}

func (srv *Server) forkExec(l listener) (*os.Process, error) {
	progName, err := exec.LookPath(os.Args[0])
	if err != nil {
		return nil, err
	}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	f, err := l.File()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	files := []*os.File{os.Stdin, os.Stdout, os.Stderr, f}
	fdEnv := fmt.Sprintf("%s=%d", FDEnvKey, len(files)-1)
	return os.StartProcess(progName, os.Args, &os.ProcAttr{
		Dir:   pwd,
		Env:   append(os.Environ(), fdEnv),
		Files: files,
	})
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

// A State represents the state of the server.
// It's used by the optional ServerState hook.
type State uint8

const (
	// StateStart represents a state that server has been started.
	StateStart State = iota

	// StateRestart represents a state that server has been restarted.
	StateRestart

	// StateShutdown represents a state that server has been shutdown.
	StateShutdown
)
