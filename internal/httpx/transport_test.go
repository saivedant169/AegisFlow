package httpx

import (
	"net/http"
	"testing"
	"time"
)

func TestClientSharesTunedTransport(t *testing.T) {
	c1 := Client(5 * time.Second)
	c2 := Client(10 * time.Second)

	if c1.Timeout != 5*time.Second || c2.Timeout != 10*time.Second {
		t.Fatal("timeouts not applied")
	}
	if c1.Transport != c2.Transport {
		t.Fatal("clients should share one transport so the connection pool is shared")
	}
	tr, ok := c1.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected an *http.Transport")
	}
	if tr.MaxIdleConnsPerHost <= 2 {
		t.Fatalf("MaxIdleConnsPerHost should be tuned above the default of 2, got %d", tr.MaxIdleConnsPerHost)
	}
}
