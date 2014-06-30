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

// Run serves the http.Handler with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func Run(addr string, timeout time.Duration, n http.Handler) {
	srv := &http.Server{Addr: addr, Handler: n}
	err := ListenAndServe(srv, timeout)

	if err != nil {
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
func ListenAndServe(srv *http.Server, timeout time.Duration) error {
	// Create the listener so we can control their lifetime
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return run(srv, l, timeout, make(chan os.Signal, 1))
}

// ListenAndServeTLS is equivalent to http.Server.ListenAndServeTLS with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func ListenAndServeTLS(srv *http.Server, certFile, keyFile string, timeout time.Duration) error {
	// Create the listener so we can control their lifetime
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
	return run(srv, tlsListener, timeout, make(chan os.Signal, 1))
}

// Serve is equivalent to http.Server.Serve with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func Serve(srv *http.Server, l net.Listener, timeout time.Duration) error {
	return run(srv, l, timeout, make(chan os.Signal, 1))
}

func run(server *http.Server, listener net.Listener, timeout time.Duration, c chan os.Signal) error {
	// Track connection state
	add := make(chan net.Conn)
	remove := make(chan net.Conn)
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateActive:
			add <- conn
		case http.StateClosed, http.StateIdle:
			remove <- conn
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

	// Set up the interrupt catch
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for _ = range c {
			server.SetKeepAlivesEnabled(false)
			listener.Close()
			signal.Stop(c)
			close(c)
		}
	}()

	// Serve with graceful listener
	err := server.Serve(listener)

	// Request done notification
	done := make(chan struct{})
	stop <- done

	if timeout > 0 {
		select {
		case <-done:
		case <-time.After(timeout):
			kill <- struct{}{}
		}
	} else {
		<-done
	}
	return err
}
