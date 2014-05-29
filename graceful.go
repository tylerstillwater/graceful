package graceful

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// Run executes negroni.Run with graceful shutdown enabled.
//
// timeout is the duration to wait until killing active requests and stopping the server.
// If timeout is 0, the server never times out. It waits for all active requests to finish.
func Run(addr string, timeout time.Duration, n http.Handler) {
	run(addr, timeout, n, make(chan os.Signal, 1))
}

func run(addr string, timeout time.Duration, n http.Handler, c chan os.Signal) {
	logger := log.New(os.Stdout, "[graceful] ", 0)
	add := make(chan net.Conn)
	remove := make(chan net.Conn)
	stop := make(chan chan struct{})
	kill := make(chan struct{})
	connections := map[net.Conn]struct{}{}

	// Create the server and listener so we can control their lifetime
	server := &http.Server{Addr: addr, Handler: n}
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal(err)
	}

	server.ConnState = func(conn net.Conn, state http.ConnState) {
		switch state {
		case http.StateActive:
			add <- conn
		case http.StateClosed, http.StateIdle:
			remove <- conn
		}
	}

	go func() {
		var done chan struct{}
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
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			server.SetKeepAlivesEnabled(false)
			listener.Close()
			signal.Stop(c)
			close(c)
		}
	}()

	server.Serve(listener)

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
}
