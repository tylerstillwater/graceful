package graceful

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/codegangsta/negroni"
)

var killTime = 50 * time.Millisecond

func runQuery(t *testing.T, expected int, shouldErr bool, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	client := http.Client{}
	r, err := client.Get("http://localhost:3000")
	if shouldErr && err == nil {
		t.Fatal("Expected an error but none was encountered.")
	} else if shouldErr && err != nil {
		if err.(*url.Error).Err == io.EOF {
			return
		}
		errno := err.(*url.Error).Err.(*net.OpError).Err.(syscall.Errno)
		if errno == syscall.ECONNREFUSED {
			return
		} else if err != nil {
			t.Fatal("Error on Get:", err)
		}
	}

	if r != nil && r.StatusCode != expected {
		t.Fatalf("Incorrect status code on response. Expected %d. Got %d", expected, r.StatusCode)
	} else if r == nil {
		t.Fatal("No response when a response was expected.")
	}
}

func TestGracefulRun(t *testing.T) {
	c := make(chan os.Signal, 1)
	n := negroni.New()
	n.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		time.Sleep(killTime / time.Duration(2.0))
		rw.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		run(":3000", killTime, n, c)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for i := 0; i < 8; i++ {
			go runQuery(t, http.StatusOK, false, &wg)
		}
		time.Sleep(10 * time.Millisecond)
		c <- os.Interrupt
		time.Sleep(10 * time.Millisecond)
		for i := 0; i < 8; i++ {
			go runQuery(t, 0, true, &wg)
		}
		wg.Done()
	}()

	wg.Wait()

}

func TestGracefulRunTimesOut(t *testing.T) {
	c := make(chan os.Signal, 1)
	n := negroni.New()
	n.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		time.Sleep(killTime * time.Duration(10.0))
		rw.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		run(":3000", killTime, n, c)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for i := 0; i < 8; i++ {
			go runQuery(t, 0, true, &wg)
		}
		time.Sleep(10 * time.Millisecond)
		c <- os.Interrupt
		time.Sleep(10 * time.Millisecond)
		for i := 0; i < 8; i++ {
			go runQuery(t, 0, true, &wg)
		}
		wg.Done()
	}()

	wg.Wait()

}

func TestGracefulRunDoesntTimeOut(t *testing.T) {
	c := make(chan os.Signal, 1)
	n := negroni.New()
	n.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		time.Sleep(killTime * time.Duration(2.0))
		rw.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		run(":3000", 0, n, c)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for i := 0; i < 8; i++ {
			go runQuery(t, http.StatusOK, false, &wg)
		}
		time.Sleep(10 * time.Millisecond)
		c <- os.Interrupt
		time.Sleep(10 * time.Millisecond)
		for i := 0; i < 8; i++ {
			go runQuery(t, 0, true, &wg)
		}
		wg.Done()
	}()

	wg.Wait()

}
