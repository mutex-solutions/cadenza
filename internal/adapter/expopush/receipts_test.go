package expopush

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

func TestCheckReceipts(t *testing.T) {
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/--/api/v2/push/getReceipts" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		if len(req.IDs) != 2 || req.IDs[0] != "a" || req.IDs[1] != "b" {
			t.Errorf("ids = %v", req.IDs)
		}
		io.WriteString(w, `{"data":{"a":{"status":"ok"},"b":{"status":"error","message":"gone","details":{"error":"DeviceNotRegistered"}}}}`)
	})
	got, err := s.CheckReceipts(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got["a"].Status != "ok" || got["a"].Gone() {
		t.Errorf("a = %+v", got["a"])
	}
	if b := got["b"]; b.Status != "error" || !b.Gone() || b.Message != "gone" {
		t.Errorf("b = %+v", b)
	}
}

func TestCheckReceiptsEmpty(t *testing.T) {
	got, err := New(Config{BaseURL: "http://127.0.0.1:1"}).CheckReceipts(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestCheckReceiptsErrors(t *testing.T) {
	_, s, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"errors":[{"code":"VALIDATION_ERROR","message":"bad ids"}]}`)
	})
	if _, err := s.CheckReceipts(context.Background(), []string{"x"}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
	_, s2, _ := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := s2.CheckReceipts(context.Background(), []string{"x"}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
