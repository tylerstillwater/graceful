package graceful

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	Timeout   time.Duration
	ConnState func(net.Conn, http.ConnState)
	interrupt chan os.Signal

	*http.Server
}

// Run serves the http.Handler with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func Run(addr string, timeout time.Duration, n http.Handler) {
	srv := &Server{
		Timeout: timeout,
		Server:  &http.Server{Addr: addr, Handler: n},
	}

	if err := srv.ListenAndServe(); err != nil {
		if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
			logger := log.New(os.Stdout, "[graceful] ", 0)
			logger.Fatal(err)
		}
	}
}

// ListenAndServe is equivalent to http.Server.ListenAndServe with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func ListenAndServe(server *http.Server, timeout time.Duration) error {
	srv := &Server{Timeout: timeout, Server: server}
	return srv.ListenAndServe()
}

func (srv *Server) ListenAndServe() error {
	// Create the listener so we can control their lifetime
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return srv.Serve(l)
}

// ListenAndServeTLS is equivalent to http.Server.ListenAndServeTLS with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func ListenAndServeTLS(server *http.Server, certFile, keyFile string, timeout time.Duration) error {
	// Create the listener ourselves so we can control its lifetime
	srv := &Server{Timeout: timeout, Server: server}
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

	conn, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	tlsListener := tls.NewListener(conn, config)
	return srv.Serve(tlsListener)
}

// Serve is equivalent to http.Server.Serve with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func Serve(server *http.Server, l net.Listener, timeout time.Duration) error {
	srv := &Server{Timeout: timeout, Server: server}
	return srv.Serve(l)
}

func (srv *Server) Serve(listener net.Listener) error {
	// Track connection state
	add := make(chan net.Conn)
	remove := make(chan net.Conn)

	srv.Server.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateActive:
			add <- conn
		case http.StateClosed, http.StateIdle:
			remove <- conn
		}

		if hook := srv.ConnState; hook != nil {
			hook(conn, state)
		}
	}

	// Manage open connections
	stop := make(chan chan struct{})
	kill := make(chan struct{})
	go func() {
		var done chan struct{}
		connections := map[net.Conn]struct{}{}
		for {
			select {
			case conn := <-add:
				connections[conn] = struct{}{}
			case conn := <-remove:
				delete(connections, conn)
				if done != nil && len(connections) == 0 {
					done <- struct{}{}
					return
				}
			case done = <-stop:
				if len(connections) == 0 {
					done <- struct{}{}
					return
				}
			case <-kill:
				for k := range connections {
					k.Close()
				}
				return
			}
		}
	}()

	if srv.interrupt == nil {
		srv.interrupt = make(chan os.Signal, 1)
	}

	// Set up the interrupt catch
	signal.Notify(srv.interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for _ = range srv.interrupt {
			srv.SetKeepAlivesEnabled(false)
			listener.Close()
			signal.Stop(srv.interrupt)
			close(srv.interrupt)
		}
	}()

	// Serve with graceful listener
	err := srv.Server.Serve(listener)

	// Request done notification
	done := make(chan struct{})
	stop <- done

	if srv.Timeout > 0 {
		select {
		case <-done:
		case <-time.After(srv.Timeout):
			kill <- struct{}{}
		}
	} else {
		<-done
	}
	return err
}
