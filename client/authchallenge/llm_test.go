package authchallenge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func llmTestImage(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 60))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeLLMAnswer(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
}

func newTestLLM(t *testing.T, baseURL string, fallback Handler) *LLMHandler {
	t.Helper()
	h, err := NewLLMHandler(LLMOptions{BaseURL: baseURL, APIKey: "test-key", Model: "test-vision"}, fallback)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	h.client.Transport = transport
	h.waitRetry = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	t.Cleanup(h.client.CloseIdleConnections)
	return h
}

func TestLLMRequestAlwaysUsesModelAcrossLogins(t *testing.T) {
	img := llmTestImage(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/gateway/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect request method, path or API headers")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-csrf-token") != "" {
			t.Error("VPN credentials sent to model")
		}
		var payload struct {
			Model          string `json:"model"`
			Stream         bool   `json:"stream"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if payload.Model != "test-vision" || payload.Stream || payload.ResponseFormat.Type != "json_object" || len(payload.Messages) != 2 {
			t.Error("incorrect model or chat request shape")
			return
		}
		if payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" {
			t.Error("incorrect message roles")
		}
		var content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL    string `json:"url"`
				Detail string `json:"detail"`
			} `json:"image_url"`
		}
		if json.Unmarshal(payload.Messages[1].Content, &content) != nil || len(content) != 2 {
			t.Error("missing image content")
			return
		}
		wantImage := "data:" + "image/png" + ";base64," + base64.StdEncoding.EncodeToString(img)
		if content[0].Type != "text" || !strings.Contains(content[0].Text, "width=100, height=60") || content[1].Type != "image_url" || content[1].ImageURL.URL != wantImage || content[1].ImageURL.Detail != "high" {
			t.Error("image bytes, dimensions or vision detail changed")
		}
		writeLLMAnswer(w, `{"coordinates":[[20,30],[50,40]],"width":100,"height":60}`)
	}))
	defer server.Close()
	manualCalls := 0
	fallback := HandlerFuncs{ClickCaptcha: func(c ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
		manualCalls++
		if !bytes.Equal(c.Image, img) || c.Context == nil {
			t.Error("fallback lost challenge or context")
		}
		return ClickCaptchaResponse{Points: []Point{{1, 2}}, Width: 100, Height: 60}, nil
	}}
	h := newTestLLM(t, server.URL+"/gateway/v1/", fallback)
	for login := 0; login < 2; login++ {
		session := h.ForLogin(context.Background())
		for attempt := 0; attempt < 5; attempt++ {
			got, err := session.HandleClickCaptcha(ClickCaptchaChallenge{Image: img})
			if err != nil {
				t.Fatal(err)
			}
			want := []Point{{20, 30}, {50, 40}}
			if !reflect.DeepEqual(got.Points, want) || got.Width != 100 || got.Height != 60 {
				t.Fatalf("response=%+v", got)
			}
		}
	}
	if calls.Load() != 10 || manualCalls != 0 {
		t.Fatalf("model=%d manual=%d", calls.Load(), manualCalls)
	}
}

func TestLLMDoesNotSaveCaptchaOrPromptForInput(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if status != http.StatusOK {
					http.Error(w, "invalid key", status)
					return
				}
				writeLLMAnswer(w, `{"coordinates":[[20,30]],"width":100,"height":60}`)
			}))
			defer server.Close()
			var output bytes.Buffer
			cli := NewCLIHandler(CLIOptions{Input: strings.NewReader(""), Output: &output})
			h := newTestLLM(t, server.URL, cli)
			path := filepath.Join(t.TempDir(), "captcha.png")
			_, err := h.HandleClickCaptcha(ClickCaptchaChallenge{Image: llmTestImage(t), OutputPath: path, Message: "Manual input must not run"})
			if (status == http.StatusOK) != (err == nil) {
				t.Fatalf("HTTP %d: err=%v", status, err)
			}
			if output.Len() != 0 {
				t.Fatal("LLM mode printed a manual input prompt or captcha URL")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("LLM mode created a captcha image: %v", err)
			}
		})
	}
}

func TestLLMCoordinateValidation(t *testing.T) {
	for _, raw := range []string{
		`{"coordinates":[],"width":100,"height":60}`,
		`{"coordinates":[[1,2]],"width":101,"height":60}`,
		`{"coordinates":[[100,2]],"width":100,"height":60}`,
		`{"coordinates":[[1,60]],"width":100,"height":60}`,
		`{"coordinates":[[-1,2]],"width":100,"height":60}`,
		`{"coordinates":[[1.1,2]],"width":100,"height":60}`,
		`{"coordinates":[[null,2]],"width":100,"height":60}`,
		`{"coordinates":[[1]],"width":100,"height":60}`,
		`{"coordinates":[[1,2,3]],"width":100,"height":60}`,
		`{"coordinates":[[1,2]],"width":100,"height":60,"extra":"x"}`,
		`{"coordinates":[[1,2]],"width":100,"height":60} {}`,
		`{"coordinates":[[1,2]]}`,
		"null", "not json",
	} {
		if _, err := parseLLMCaptchaResponse(raw, 100, 60); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	got, err := parseLLMCaptchaResponse(`{"coordinates":[[0,0],[99,59]],"width":100,"height":60}`, 100, 60)
	if err != nil || !reflect.DeepEqual(got.Points, []Point{{0, 0}, {99, 59}}) {
		t.Fatalf("boundary coordinates=%+v, %v", got, err)
	}
}

func TestLLMFailuresReturnErrorsWithoutManualInputOrLeakingBody(t *testing.T) {
	for _, mode := range []string{"bad-request", "unauthorized", "forbidden", "rate-limit", "gateway-timeout", "redirect", "malformed", "invalid-coordinates", "empty", "refusal", "truncated", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/chat/completions" {
					t.Error("retry followed a redirect instead of using the original endpoint")
				}
				switch mode {
				case "bad-request":
					http.Error(w, "test-key private response", 400)
				case "unauthorized":
					http.Error(w, "test-key private response", 401)
				case "forbidden":
					http.Error(w, "test-key private response", 403)
				case "rate-limit":
					http.Error(w, "test-key private response", 429)
				case "gateway-timeout":
					http.Error(w, "test-key private response", 524)
				case "redirect":
					http.Redirect(w, r, "/must-not-follow", 302)
				case "malformed":
					fmt.Fprint(w, "test-key private response")
				case "invalid-coordinates":
					writeLLMAnswer(w, `{"coordinates":[[500,1]],"width":100,"height":60}`)
				case "empty":
					writeLLMAnswer(w, `{"coordinates":[],"width":100,"height":60}`)
				case "refusal":
					fmt.Fprint(w, `{"choices":[{"message":{"refusal":"test-key private response"}}]}`)
				case "truncated":
					fmt.Fprint(w, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`)
				case "oversized":
					fmt.Fprint(w, strings.Repeat("x", maxLLMResponseBytes+1))
				}
			}))
			defer server.Close()
			img := llmTestImage(t)
			manual := 0
			h := newTestLLM(t, server.URL, HandlerFuncs{ClickCaptcha: func(c ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
				manual++
				if !bytes.Equal(c.Image, img) {
					t.Error("fallback lost original image")
				}
				return ClickCaptchaResponse{Points: []Point{{1, 2}}}, nil
			}})
			_, err := h.HandleClickCaptcha(ClickCaptchaChallenge{Image: img})
			if err == nil {
				t.Fatal("model failure must stop captcha authentication")
			}
			if strings.Contains(err.Error(), "test-key") || strings.Contains(err.Error(), "private response") {
				t.Fatal("sensitive content in returned error")
			}
			wantCalls := int32(6)
			if mode == "refusal" || mode == "oversized" {
				wantCalls = 1
			}
			if calls.Load() != wantCalls || manual != 0 {
				t.Fatalf("model=%d manual=%d", calls.Load(), manual)
			}
		})
	}
}

type llmBlockingTransport struct{ started chan struct{} }

func (b llmBlockingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestLLMCancellationAndTimeout(t *testing.T) {
	for _, mode := range []string{"cancelled-before", "cancelled-during", "login-deadline", "model-timeout"} {
		t.Run(mode, func(t *testing.T) {
			var manual atomic.Int32
			h := newTestLLM(t, "https://model.example/v1", HandlerFuncs{ClickCaptcha: func(ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
				manual.Add(1)
				return ClickCaptchaResponse{}, nil
			}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "login-deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			}
			started := make(chan struct{})
			h.client.Transport = llmBlockingTransport{started: started}
			if mode == "model-timeout" {
				h.client.Timeout = 20 * time.Millisecond
			}
			if mode == "cancelled-before" {
				cancel()
			}
			img := llmTestImage(t)
			done := make(chan error, 1)
			go func() { _, err := h.ForLogin(ctx).HandleClickCaptcha(ClickCaptchaChallenge{Image: img}); done <- err }()
			if mode == "cancelled-during" {
				select {
				case <-started:
					cancel()
				case <-time.After(time.Second):
					cancel()
					<-done
					t.Fatal("request did not start")
				}
			}
			select {
			case err := <-done:
				if mode == "model-timeout" {
					if err == nil || manual.Load() != 0 {
						t.Fatalf("timeout must fail without manual input: %v, manual=%d", err, manual.Load())
					}
				} else if (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) || manual.Load() != 0 {
					t.Fatalf("cancel: %v manual=%d", err, manual.Load())
				}
			case <-time.After(time.Second):
				cancel()
				<-done
				t.Fatal("request did not stop")
			}
		})
	}
}

func TestLLMOtherChallengesUseFallback(t *testing.T) {
	calls := 0
	want := errors.New("manual sentinel")
	h := newTestLLM(t, "https://model.example/v1", HandlerFuncs{
		Code:        func(CodeChallenge) (CodeResponse, error) { calls++; return CodeResponse{}, want },
		TextCaptcha: func(TextCaptchaChallenge) (TextCaptchaResponse, error) { calls++; return TextCaptchaResponse{}, want },
		ExternalLogin: func(ExternalLoginChallenge) (ExternalLoginResponse, error) {
			calls++
			return ExternalLoginResponse{}, want
		},
	})
	if _, err := h.HandleCodeChallenge(CodeChallenge{}); err != want {
		t.Fatal(err)
	}
	if _, err := h.HandleTextCaptcha(TextCaptchaChallenge{}); err != want {
		t.Fatal(err)
	}
	if _, err := h.HandleExternalLogin(ExternalLoginChallenge{}); err != want {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}

func TestLLMEndpoint(t *testing.T) {
	for base, want := range map[string]string{
		"https://model.example":                     "https://model.example/v1/chat/completions",
		"https://model.example/v1/":                 "https://model.example/v1/chat/completions",
		"https://model.example/custom/v1":           "https://model.example/custom/v1/chat/completions",
		"http://127.0.0.1:1234/v1/chat/completions": "http://127.0.0.1:1234/v1/chat/completions",
	} {
		if got, err := llmEndpoint(base); err != nil || got != want {
			t.Errorf("%q => %q %v", base, got, err)
		}
	}
	for _, base := range []string{"", "model.example", "ftp://model.example", "https://user:secret@model.example", "https://model.example?key=secret", "https://model.example#secret", "https://%"} {
		if _, err := llmEndpoint(base); err == nil {
			t.Errorf("accepted %q", base)
		}
	}
}

func TestCaptchaFallbackClosesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := captchaTestOutput{writes: make(chan string, 128)}
	done := make(chan error, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		_, err := serveCaptchaContext(ctx, []byte("image"), time.Minute, output, "linux", func(string, io.Writer) { t.Error("Linux opened browser") })
		done <- err
	}()
	t.Cleanup(func() { cancel(); <-serverDone })
	localURL := waitCaptchaURL(t, output.writes)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("captcha server did not stop")
	}
	client := captchaHTTPClient()
	defer client.CloseIdleConnections()
	if _, err := client.Get(localURL); err == nil {
		t.Fatal("captcha listener still open")
	}
}
