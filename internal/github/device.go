package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCodeURL  = "https://github.com/login/device/code"
	defaultTokenURL = "https://github.com/login/oauth/access_token"
	defaultInterval = 5 * time.Second
)

var (
	// ErrAuthPending means the user has not yet authorized the device code.
	ErrAuthPending = errors.New("authorization pending")
	// ErrAccessDenied means the user declined authorization.
	ErrAccessDenied = errors.New("authorization denied")
	// ErrDeviceCodeExpired means the device code timed out.
	ErrDeviceCodeExpired = errors.New("device code expired")
)

// DeviceCode is the payload printed to the user and then polled.
type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// DeviceFlow talks to GitHub's device-authorization endpoints. HTTP, URLs and
// Sleep are injectable so tests never hit the network.
type DeviceFlow struct {
	HTTP     *http.Client
	CodeURL  string
	TokenURL string
	Sleep    func(time.Duration)
	now      func() time.Time
}

// NewDeviceFlow returns a flow aimed at api.github.com's login hosts.
func NewDeviceFlow(httpClient *http.Client) *DeviceFlow {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &DeviceFlow{
		HTTP:     httpClient,
		CodeURL:  defaultCodeURL,
		TokenURL: defaultTokenURL,
		Sleep:    time.Sleep,
		now:      time.Now,
	}
}

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	Error                   string `json:"error"`
	ErrorDescription        string `json:"error_description"`
}

// RequestCode starts device authorization and returns the user-facing code.
func (f *DeviceFlow) RequestCode(ctx context.Context, clientID string) (DeviceCode, error) {
	if clientID == "" {
		return DeviceCode{}, errors.New("OAuth client ID is empty")
	}

	form := url.Values{
		"client_id": {clientID},
		"scope":     {Scope},
	}
	var parsed deviceCodeResponse
	if err := f.postForm(ctx, f.codeURL(), form, &parsed); err != nil {
		return DeviceCode{}, err
	}
	if parsed.Error != "" {
		return DeviceCode{}, deviceError(parsed.Error, parsed.ErrorDescription)
	}
	if parsed.DeviceCode == "" || parsed.UserCode == "" {
		return DeviceCode{}, errors.New("GitHub device code response was missing device_code or user_code")
	}

	interval := time.Duration(parsed.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	expires := time.Duration(parsed.ExpiresIn) * time.Second
	uri := parsed.VerificationURI
	if uri == "" {
		uri = parsed.VerificationURIComplete
	}

	return DeviceCode{
		DeviceCode:      parsed.DeviceCode,
		UserCode:        parsed.UserCode,
		VerificationURI: uri,
		ExpiresIn:       expires,
		Interval:        interval,
	}, nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Interval         int    `json:"interval"`
}

// WaitForToken polls until the user authorizes, the code expires, or ctx is
// cancelled. It never logs the access token.
func (f *DeviceFlow) WaitForToken(ctx context.Context, clientID string, code DeviceCode) (string, error) {
	if f.Sleep == nil {
		f.Sleep = time.Sleep
	}
	now := f.now
	if now == nil {
		now = time.Now
	}

	deadline := now().Add(code.ExpiresIn)
	if code.ExpiresIn <= 0 {
		deadline = now().Add(15 * time.Minute)
	}
	interval := code.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	form := url.Values{
		"client_id":   {clientID},
		"device_code": {code.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !now().Before(deadline) {
			return "", ErrDeviceCodeExpired
		}

		var parsed tokenResponse
		if err := f.postForm(ctx, f.tokenURL(), form, &parsed); err != nil {
			return "", err
		}

		switch {
		case parsed.AccessToken != "":
			return parsed.AccessToken, nil
		case parsed.Error == "authorization_pending":
			f.Sleep(interval)
		case parsed.Error == "slow_down":
			interval += 5 * time.Second
			if parsed.Interval > 0 {
				interval = time.Duration(parsed.Interval) * time.Second
			}
			f.Sleep(interval)
		case parsed.Error == "expired_token" || parsed.Error == "expired":
			return "", ErrDeviceCodeExpired
		case parsed.Error == "access_denied":
			return "", ErrAccessDenied
		case parsed.Error != "":
			return "", deviceError(parsed.Error, parsed.ErrorDescription)
		default:
			return "", errors.New("GitHub token response had neither access_token nor error")
		}
	}
}

func (f *DeviceFlow) postForm(ctx context.Context, endpoint string, form url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := f.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub device flow: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading GitHub device flow response: %w", err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decoding GitHub device flow response: %w", err)
	}
	return nil
}

func (f *DeviceFlow) codeURL() string {
	if f.CodeURL == "" {
		return defaultCodeURL
	}
	return f.CodeURL
}

func (f *DeviceFlow) tokenURL() string {
	if f.TokenURL == "" {
		return defaultTokenURL
	}
	return f.TokenURL
}

func deviceError(code, description string) error {
	switch code {
	case "authorization_pending":
		return ErrAuthPending
	case "access_denied":
		return ErrAccessDenied
	case "expired_token", "expired":
		return ErrDeviceCodeExpired
	}
	if description != "" {
		return fmt.Errorf("GitHub device flow: %s (%s)", code, description)
	}
	return fmt.Errorf("GitHub device flow: %s", code)
}
