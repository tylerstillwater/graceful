package graceful

import (
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/codegangsta/negroni"
)

func runQuery(t *testing.T, expected int) {
	r, err := http.Get("http://localhost:3000")
	if err != nil {
		t.Fatal("Get failed:", err)
	}
	if r.StatusCode != expected {
		t.Fatalf("Incorrect status code on response. Expected %d. Got %d", expected, r.StatusCode)
	}
}

func TestGraceful(t *testing.T) {

	n := negroni.New()
	n.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		rw.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		Run(":3000", 10*time.Millisecond, n)
		wg.Done()
	}()

	go func() {
		runQuery(t, http.StatusOK)
		c <- os.Interrupt
		runQuery(t, http.StatusServiceUnavailable)
		runQuery(t, http.StatusServiceUnavailable)
		runQuery(t, http.StatusServiceUnavailable)
	}()

	wg.Wait()

}
