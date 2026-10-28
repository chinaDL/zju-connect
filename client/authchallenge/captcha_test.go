package authchallenge

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type captchaTestOutput struct {
	writes chan string
}

func (o captchaTestOutput) Write(p []byte) (int, error) {
	o.writes <- string(p)
	return len(p), nil
}

type captchaResult struct {
	response ClickCaptchaResponse
	err      error
}

func waitCaptchaURL(t *testing.T, output <-chan string) string {
	t.Helper()
	select {
	case line := <-output:
		const prefix = "Captcha local URL: "
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("first output = %q", line)
		}
		return strings.TrimSpace(strings.TrimPrefix(line, prefix))
	case <-time.After(3 * time.Second):
		t.Fatal("captcha server did not print its URL")
		return ""
	}
}

func captchaHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   time.Second,
		Transport: &http.Transport{Proxy: nil},
	}
}

func TestServeCaptchaRemoteURLsAndSubmission(t *testing.T) {
	output := captchaTestOutput{writes: make(chan string, 128)}
	result := make(chan captchaResult, 1)
	serverDone := make(chan struct{})
	image := []byte("captcha image")
	go func() {
		defer close(serverDone)
		response, err := serveCaptcha(image, 2*time.Second, output, "linux", func(string, io.Writer) {
			t.Error("Linux must not open a browser")
		})
		result <- captchaResult{response, err}
	}()
	t.Cleanup(func() { <-serverDone })

	localURL := waitCaptchaURL(t, output.writes)
	parsed, err := url.Parse(localURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "127.0.0.1" || !strings.HasPrefix(parsed.Path, "/captcha/") || !strings.HasSuffix(parsed.Path, "/") {
		t.Fatalf("unexpected local URL: %s", localURL)
	}
	client := captchaHTTPClient()
	defer client.CloseIdleConnections()
	for _, path := range []string{"/", "/captcha.img", "/submit", "/captcha/wrong/"} {
		resp, err := client.Get(parsed.Scheme + "://" + parsed.Host + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("unauthorized path %q returned %d", path, resp.StatusCode)
		}
	}

	page, err := client.Get(localURL)
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := io.ReadAll(page.Body)
	_ = page.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || !strings.Contains(string(pageBody), `src="captcha.img"`) || !strings.Contains(string(pageBody), `xhr.open('POST', 'submit'`) {
		t.Fatalf("captcha page did not use token-scoped routes: status=%d", page.StatusCode)
	}
	if page.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", page.Header.Get("Referrer-Policy"))
	}
	imageResponse, err := client.Get(localURL + "captcha.img")
	if err != nil {
		t.Fatal(err)
	}
	gotImage, err := io.ReadAll(imageResponse.Body)
	_ = imageResponse.Body.Close()
	if err != nil || imageResponse.StatusCode != http.StatusOK || string(gotImage) != string(image) {
		t.Fatalf("captcha image: status=%d body=%q err=%v", imageResponse.StatusCode, gotImage, err)
	}

	bad, err := client.PostForm(localURL+"submit", url.Values{"code": {`{"coordinates":"bad"}`}})
	if err != nil {
		t.Fatal(err)
	}
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid submission status = %d", bad.StatusCode)
	}
	good, err := client.PostForm(localURL+"submit", url.Values{"code": {`{"coordinates":[[11,22]],"width":300,"height":150}`}})
	if err != nil {
		t.Fatal(err)
	}
	_ = good.Body.Close()
	if good.StatusCode != http.StatusOK {
		t.Fatalf("valid submission status = %d", good.StatusCode)
	}
	got := <-result
	if got.err != nil || len(got.response.Points) != 1 || got.response.Points[0] != (Point{X: 11, Y: 22}) {
		t.Fatalf("captcha result = %+v, err=%v", got.response, got.err)
	}
	if _, err := client.Get(localURL); err == nil {
		t.Fatal("captcha listener remained open after submission")
	}
	addresses, err := activeIPv4Addresses()
	if err != nil {
		t.Fatal(err)
	}
	printed := ""
	for len(output.writes) > 0 {
		printed += <-output.writes
	}
	for _, addr := range addresses {
		want := "Captcha network URL: http://" + addr + ":" + parsed.Port() + parsed.Path
		if !strings.Contains(printed, want) {
			t.Fatalf("missing network URL %q in %q", want, printed)
		}
	}
}

func TestServeCaptchaTimeoutAndBrowserPolicy(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			output := captchaTestOutput{writes: make(chan string, 128)}
			opened := ""
			_, err := serveCaptcha([]byte("image"), 20*time.Millisecond, output, platform, func(url string, _ io.Writer) {
				opened = url
			})
			if err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("timeout error = %v", err)
			}
			localURL := waitCaptchaURL(t, output.writes)
			if platform == "linux" && opened != "" {
				t.Fatalf("Linux opened browser: %s", opened)
			}
			if platform != "linux" && opened != localURL {
				t.Fatalf("%s opened %q, want %q", platform, opened, localURL)
			}
			client := captchaHTTPClient()
			defer client.CloseIdleConnections()
			if _, err := client.Get(localURL); err == nil {
				t.Fatal("captcha listener remained open after timeout")
			}
		})
	}
}
