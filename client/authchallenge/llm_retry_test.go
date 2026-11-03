package authchallenge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestLLMRetryRecoveryAndLimit(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusFound, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable, 524} {
		for _, failures := range []int{0, 1, 5, 6} {
			t.Run(fmt.Sprintf("HTTP%d/failures%d", status, failures), func(t *testing.T) {
				var calls atomic.Int32
				var firstBody string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					n := int(calls.Add(1))
					if n == 1 {
						firstBody = string(body)
					} else if string(body) != firstBody {
						t.Error("retry changed image or request")
					}
					if n <= failures {
						http.Error(w, "model request failed", status)
						return
					}
					writeLLMAnswer(w, `{"coordinates":[[1,2]],"width":100,"height":60}`)
				}))
				defer server.Close()
				manual := 0
				h := newTestLLM(t, server.URL, HandlerFuncs{ClickCaptcha: func(ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
					manual++
					return ClickCaptchaResponse{}, nil
				}})
				var delays []time.Duration
				var retries []int
				h.waitRetry = func(ctx context.Context, delay time.Duration) error { delays = append(delays, delay); return ctx.Err() }
				h.options.OnRetry = func(n int, delay time.Duration, err error) {
					retries = append(retries, n)
					if err == nil || delay <= 0 {
						t.Error("missing retry reason or delay")
					}
				}
				got, err := h.HandleClickCaptcha(ClickCaptchaChallenge{Image: llmTestImage(t)})
				if failures < 6 && err != nil {
					t.Fatal(err)
				}
				wantRetries := min(failures, 5)
				if calls.Load() != int32(wantRetries+1) || len(retries) != wantRetries || len(delays) != wantRetries {
					t.Fatalf("calls=%d retries=%v delays=%v", calls.Load(), retries, delays)
				}
				wantDelays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
				for i := range retries {
					if retries[i] != i+1 || delays[i] != wantDelays[i] {
						t.Fatal("incorrect retry numbering or backoff")
					}
				}
				if failures == 6 {
					if err == nil || manual != 0 {
						t.Fatal("exhaustion must return an error without manual input")
					}
				} else if manual != 0 || !reflect.DeepEqual(got.Points, []Point{{1, 2}}) {
					t.Fatalf("recovered response=%+v manual=%d", got, manual)
				}
			})
		}
	}
}

func TestLLMRetryBackoffCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "busy", 429) }))
	defer server.Close()
	manual := 0
	h := newTestLLM(t, server.URL, HandlerFuncs{ClickCaptcha: func(ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
		manual++
		return ClickCaptchaResponse{}, nil
	}})
	h.waitRetry = waitLLMRetry
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.options.OnRetry = func(int, time.Duration, error) { cancel() }
	started := time.Now()
	_, err := h.ForLogin(ctx).HandleClickCaptcha(ClickCaptchaChallenge{Image: llmTestImage(t)})
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 || manual != 0 {
		t.Fatalf("err=%v calls=%d manual=%d", err, calls.Load(), manual)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancel did not interrupt backoff")
	}
}
