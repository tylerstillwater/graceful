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
	active := make(chan int)
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
		for {
			select {
			case conn := <-add:
				connections[conn] = struct{}{}
				count := len(connections)
				go func() {
					active <- count
				}()
			case conn := <-remove:
				delete(connections, conn)
				count := len(connections)
				go func() {
					active <- count
				}()
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

	if timeout > 0 {
		kill := time.NewTimer(timeout)
		for {
			select {
			case count := <-active:
				if count == 0 {
					return
				}
			case <-kill.C:
				for k := range connections {
					k.Close()
				}
				return
			}
		}
	} else {
		for count := range active {
			if count == 0 {
				return
			}
		}
	}

}
