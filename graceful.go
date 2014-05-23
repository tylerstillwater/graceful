package graceful

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/codegangsta/negroni"
)

type graceful struct {
	closing bool
	timeout time.Duration
}

// ServeHTTP checks to see if we are in a closing state. If we are, it responds with a 503.
// If not, it simply moves on to the next handler.
func (g *graceful) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if g.closing {
		rw.WriteHeader(http.StatusServiceUnavailable)
	} else {
		next(rw, r)
	}
}

// c is defined here to facilitate testing
var c = make(chan os.Signal, 1)

// Run executes negroni.Run with graceful shutdown enabled.
//
// timeout is the duration to wait until killing in-flight requests and stopping the server.
func Run(addr string, timeout time.Duration, toRun *negroni.Negroni) {
	logger := log.New(os.Stdout, "[negroni] ", 0)

	// Inject our graceful shutdown middleware at the top of the stack
	gracefulHandler := &graceful{closing: false, timeout: timeout}

	n := negroni.New()
	n.Use(gracefulHandler)
	n.UseHandler(toRun)

	// Create the server and listener so we can control their lifetime
	server := &http.Server{Addr: addr, Handler: n}
	if addr == "" {
		addr = ":http"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal(err)
	}

	// Set up the interrupt catch
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			gracefulHandler.closing = true
			time.Sleep(timeout)
			listener.Close()
		}
	}()

	server.Serve(listener)

}
