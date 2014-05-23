graceful
========

Graceful is a Go package enabling graceful shutdown of [Negroni](https://github.com/codegangsta/negroni) servers.

[![wercker status](https://app.wercker.com/status/2729ba763abf87695a17547e0f7af4a4/m "wercker status")](https://app.wercker.com/project/bykey/2729ba763abf87695a17547e0f7af4a4)

## Usage

Usage of Graceful is simple. Simply create your [Negroni](https://github.com/codegangsta/negroni) stack as usual, then call
the `Run` function provided by graceful instead of the `Run` function provided by Negroni:

```go
package main

import (
  "github.com/codegangsta/negroni"
  "github.com/stretchr/graceful"
  "net/http"
  "fmt"
)

func main() {
  mux := http.NewServeMux()
  mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
    fmt.Fprintf(w, "Welcome to the home page!")
  })

  n := negroni.Classic()
  n.UseHandler(mux)
  //n.Run(":3000")
  graceful.Run(":3001",10*time.Second,n)
}
```

When Graceful is sent a SIGINT (ctrl+c), it:

1. Disables keepalive connections.
2. Starts a timer of `timeout` duration to give in-flight requests a chance to finish.
3. Responds with `503 Service Unavailable` to any new requests.
4. Closes the server when `timeout` has passed, terminating any remaining connections.

## Notes

Graceful relies on functionality in [Go 1.3](http://tip.golang.org/doc/go1.3) which has not yet been released. If you wish to use it, you
must [install the beta](https://code.google.com/p/go/wiki/Downloads) of Go 1.3. Once 1.3 is released, this note will be removed.


Please see the [Graceful Documentation](https://godoc.org/github.com/stretchr/graceful) on godoc for more information.
