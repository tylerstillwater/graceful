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

func runServer(timeout, sleep time.Duration, c chan os.Signal) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		time.Sleep(sleep)
		rw.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: ":3000", Handler: mux}
	l, err := net.Listen("tcp", ":3000")
	if err != nil {
		return err
	}
	return run(srv, l, timeout, c)
}

func TestGracefulRun(t *testing.T) {
	c := make(chan os.Signal, 1)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		runServer(killTime, killTime/2, c)
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

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		runServer(killTime, killTime*10, c)
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

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		runServer(0, killTime*2, c)
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

func TestGracefulRunNoRequests(t *testing.T) {
	c := make(chan os.Signal, 1)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		runServer(0, killTime*2, c)
		wg.Done()
	}()

	c <- os.Interrupt

	wg.Wait()

}
