package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mythologyli/zju-connect/client/authchallenge"
)

func TestLoginLLMCaptchaOnlyUsesModelAcrossLogins(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var images [][]byte
	for i := 0; i < maxAttempts; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 100, 60))
		img.Set(0, 0, color.RGBA{R: uint8(i + 1), A: 255})
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		images = append(images, b.Bytes())
	}
	for _, tt := range []struct {
		name                                        string
		acceptAt, wantModel, wantManual, wantImages int
	}{
		{"automatic success", 1, 1, 0, 1},
		{"two rejected then model succeeds", 3, 3, 0, 3},
		{"four rejected then model succeeds", 5, 5, 0, 5},
		{"total limit preserved without manual input", 0, 5, 0, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var imageCalls, modelCalls, submissions atomic.Int32
			vpn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/passport/v1/public/authConfig":
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
						"isLogin": 0, "csrfToken": "local-csrf", "pubKey": key.N.Text(16), "pubKeyExp": strconv.Itoa(key.E), "antiReplayRand": "local-nonce",
						"authServerInfoList": []AuthInfo{{AuthType: "auth/psw", LoginDomain: "local"}},
					}})
				case "/passport/v1/public/checkCode":
					n := int(imageCalls.Add(1))
					if n > len(images) {
						t.Error("too many new captchas")
						http.Error(w, "too many", 500)
						return
					}
					_, _ = w.Write(images[n-1])
				case "/passport/v1/auth/psw":
					var request struct {
						GraphCheckCode string `json:"graphCheckCode"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						http.Error(w, "bad request", 400)
						return
					}
					if request.GraphCheckCode != "" {
						n := int(submissions.Add(1))
						var answer graphCheckCodePayload
						if json.Unmarshal([]byte(request.GraphCheckCode), &answer) != nil || answer.Width != 100 || answer.Height != 60 || len(answer.Coordinates) != 1 || len(answer.Coordinates[0]) != 2 || answer.Coordinates[0][0] != int(imageCalls.Load()) {
							t.Error("submitted coordinates do not match current image")
						}
						if tt.acceptAt > 0 && n == tt.acceptAt {
							http.SetCookie(w, &http.Cookie{Name: "sid", Value: "local-test-session", Path: "/"})
							fmt.Fprint(w, `{"code":0,"data":{"ticket":"local-ticket","graphCheckCodeEnable":0}}`)
							return
						}
					}
					fmt.Fprint(w, `{"code":401,"message":"captcha required","data":{"graphCheckCodeEnable":1}}`)
				case "/controller/v1/public/reportEnv", "/passport/v1/auth/authCheck":
					fmt.Fprint(w, `{"code":0,"data":{}}`)
				case "/passport/v1/user/onlineInfo":
					fmt.Fprint(w, `{"code":0,"data":{"username":"test-user"}}`)
				default:
					t.Errorf("unexpected VPN mock path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer vpn.Close()
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelCalls.Add(1)
				if r.Header.Get("Cookie") != "" || r.Header.Get("x-csrf-token") != "" {
					t.Error("VPN credentials leaked to model")
				}
				var request struct {
					Messages []struct {
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
					t.Error("invalid model request")
					http.Error(w, "bad request", 400)
					return
				}
				var content []struct {
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if json.Unmarshal(request.Messages[1].Content, &content) != nil || len(content) != 2 {
					t.Error("missing image")
					http.Error(w, "bad image", 400)
					return
				}
				parts := strings.SplitN(content[1].ImageURL.URL, ",", 2)
				if len(parts) != 2 {
					t.Error("missing data URL")
					return
				}
				img, err := base64.StdEncoding.DecodeString(parts[1])
				n := int(imageCalls.Load())
				if err != nil || n < 1 || n > len(images) || !bytes.Equal(img, images[n-1]) {
					t.Error("model did not receive freshly fetched image")
					return
				}
				answer := fmt.Sprintf(`{"coordinates":[[%d,20]],"width":100,"height":60}`, n)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": answer}}}})
			}))
			defer model.Close()
			manual := 0
			h, err := authchallenge.NewLLMHandler(authchallenge.LLMOptions{BaseURL: model.URL, APIKey: "mock-key", Model: "mock-vision"}, authchallenge.HandlerFuncs{
				ClickCaptcha: func(c authchallenge.ClickCaptchaChallenge) (authchallenge.ClickCaptchaResponse, error) {
					manual++
					n := int(imageCalls.Load())
					if n < 1 || n > len(images) || !bytes.Equal(c.Image, images[n-1]) || c.Context == nil {
						t.Fatal("manual fallback lost current image or context")
					}
					return authchallenge.ClickCaptchaResponse{Points: []authchallenge.Point{{X: n, Y: 20}}, Width: 100, Height: 60}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			session := newTLSTestSession(vpn)
			defer session.client.CloseIdleConnections()
			for login := 0; login < 2; login++ {
				imageCalls.Store(0)
				modelCalls.Store(0)
				submissions.Store(0)
				manual = 0
				result, err := session.Login(PasswordLogin{Username: "test-user", Password: "test-password", Domain: "local"}, LoginOptions{DeviceID: "local-test-device", ChallengeHandler: h})
				if tt.acceptAt == 0 {
					if err == nil || !strings.Contains(err.Error(), "after 5 attempts") {
						t.Fatalf("total attempts: %v", err)
					}
				} else if err != nil || result.SID != "local-test-session" {
					t.Fatalf("login=%+v err=%v", result, err)
				}
				if int(modelCalls.Load()) != tt.wantModel || manual != tt.wantManual || int(imageCalls.Load()) != tt.wantImages || int(submissions.Load()) != tt.wantImages {
					t.Fatalf("login %d: model=%d manual=%d images=%d submissions=%d", login, modelCalls.Load(), manual, imageCalls.Load(), submissions.Load())
				}
			}
		})
	}
}
