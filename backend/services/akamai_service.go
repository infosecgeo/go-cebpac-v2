package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	hyper "github.com/Hyper-Solutions/hyper-sdk-go/v2"
	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/cookiejar"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"cebupac/backend/config"
	"cebupac/backend/logger"
	proxymgr "cebupac/backend/proxy"
)

// AkamaiClient contains a ready-to-use challenged client and its cookies.
type AkamaiClient struct {
	Client    tls_client.HttpClient
	Jar       *cookiejar.Jar
	ProxyID   string
	CreatedAt time.Time
}

// AkamaiService encapsulates Akamai bypass setup and reusable challenged clients.
type AkamaiService struct {
	cfg          *config.Config
	logger       *logger.Logger
	proxyManager *proxymgr.Manager
	mu           sync.Mutex
	cached       []*AkamaiClient
	cacheTTL     time.Duration
}

// NewAkamaiService creates an Akamai service with optional proxy rotation.
func NewAkamaiService(cfg *config.Config, proxyManager *proxymgr.Manager) *AkamaiService {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &AkamaiService{
		cfg:          cfg,
		logger:       logger.GetLogger(),
		proxyManager: proxyManager,
		cached:       make([]*AkamaiClient, 0, 2),
		cacheTTL:     90 * time.Second,
	}
}

// AcquireClient returns a challenged client, reusing a recent one when possible.
func (s *AkamaiService) AcquireClient(ctx context.Context) (*AkamaiClient, error) {
	s.mu.Lock()
	now := time.Now().UTC()
	for len(s.cached) > 0 {
		candidate := s.cached[len(s.cached)-1]
		s.cached = s.cached[:len(s.cached)-1]
		if now.Sub(candidate.CreatedAt) <= s.cacheTTL {
			s.mu.Unlock()
			return candidate, nil
		}
	}
	s.mu.Unlock()

	attempts := 1
	if s.proxyManager != nil && s.cfg.Proxy.Enabled {
		attempts = s.proxyManager.AvailableCount()
		if attempts <= 0 {
			attempts = 1
		}
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		proxyID := ""
		proxyURL := ""
		if s.proxyManager != nil && s.cfg.Proxy.Enabled {
			record, err := s.proxyManager.NextProxy(ctx)
			if err != nil {
				return nil, err
			}
			proxyID = record.ID
			proxyURL = record.Address
		}
		client, err := s.runChallenge(ctx, proxyURL)
		if err != nil {
			lastErr = err
			if proxyID != "" {
				s.proxyManager.MarkFailed(ctx, proxyID)
			}
			continue
		}
		client.ProxyID = proxyID
		if proxyID != "" {
			s.proxyManager.MarkSucceeded(ctx, proxyID)
		}
		return client, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unable to acquire Akamai client")
	}
	return nil, lastErr
}

// ReleaseClient returns a challenged client to a short-lived cache.
func (s *AkamaiService) ReleaseClient(client *AkamaiClient) {
	if client == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client.CreatedAt = time.Now().UTC()
	s.cached = append(s.cached, client)
	if len(s.cached) > 2 {
		s.cached = s.cached[len(s.cached)-2:]
	}
}

func (s *AkamaiService) runChallenge(ctx context.Context, proxyURL string) (*AkamaiClient, error) {
	jar, _ := cookiejar.New(nil)
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(s.requestTimeoutSeconds()),
		tls_client.WithClientProfile(profiles.Chrome_133),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCookieJar(jar),
		tls_client.WithRandomTLSExtensionOrder(),
	}
	if strings.TrimSpace(proxyURL) != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}

	baseURL := s.cfg.Akamai.BaseURL
	if baseURL == "" {
		baseURL = s.cfg.CebupacificAir.BaseURL
	}
	userAgent := s.cfg.Akamai.UserAgent
	secChUA := s.cfg.Akamai.SecChUa
	acceptLang := s.cfg.CebupacificAir.AcceptLang

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"sec-ch-ua":                 {secChUA},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Windows"`},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {userAgent},
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"sec-fetch-site":            {"none"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-user":            {"?1"},
		"sec-fetch-dest":            {"document"},
		"accept-encoding":           {"gzip, deflate, br, zstd"},
		"accept-language":           {acceptLang},
		"priority":                  {"u=0, i"},
		http.HeaderOrderKey: {
			"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
			"upgrade-insecure-requests", "user-agent", "accept",
			"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
			"sec-fetch-dest", "accept-encoding", "accept-language", "priority",
		},
		http.PHeaderOrderKey: {":method", ":authority", ":scheme", ":path"},
	}

	var resp *http.Response
	for attempt := 0; attempt < 4; attempt++ {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("homepage request: %w", err)
	}
	defer resp.Body.Close()
	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	htmlBody := string(htmlBytes)

	allSrcRe := regexp.MustCompile(`src=["']([^"']+)["']`)
	var sbsdURL, v3URL string
	for _, match := range allSrcRe.FindAllStringSubmatch(htmlBody, -1) {
		src := match[1]
		if strings.Contains(src, "?v=") {
			if strings.HasPrefix(src, "/") {
				sbsdURL = baseURL + src
			} else {
				sbsdURL = src
			}
			continue
		}
		if v3URL == "" {
			if strings.HasPrefix(src, "/") {
				v3URL = baseURL + src
			} else if strings.HasPrefix(src, "http") {
				v3URL = src
			}
		}
	}
	if sbsdURL == "" {
		return nil, fmt.Errorf("SBSD script not found")
	}

	parsedSBSD, _ := url.Parse(sbsdURL)
	vValue := parsedSBSD.Query().Get("v")
	scriptReq, err := http.NewRequestWithContext(ctx, http.MethodGet, sbsdURL, nil)
	if err != nil {
		return nil, err
	}
	scriptReq.Header = http.Header{
		"sec-ch-ua-platform": {`"Windows"`},
		"user-agent":         {userAgent},
		"sec-ch-ua":          {secChUA},
		"sec-ch-ua-mobile":   {"?0"},
		"accept":             {"*/*"},
		"sec-fetch-site":     {"same-origin"},
		"sec-fetch-mode":     {"no-cors"},
		"sec-fetch-dest":     {"script"},
		"referer":            {baseURL + "/"},
		"accept-encoding":    {"gzip, deflate, br, zstd"},
		"accept-language":    {acceptLang},
	}
	scriptResp, err := client.Do(scriptReq)
	if err != nil {
		return nil, err
	}
	sbsdScriptBytes, _ := io.ReadAll(scriptResp.Body)
	scriptResp.Body.Close()
	sbsdScript := string(sbsdScriptBytes)
	oCookie := cookieValue(jar, baseURL, "bm_so")
	if oCookie == "" {
		oCookie = cookieValue(jar, baseURL, "sbsd_o")
	}
	ipReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return nil, err
	}
	ipResp, err := client.Do(ipReq)
	if err != nil {
		return nil, err
	}
	ipBytes, _ := io.ReadAll(ipResp.Body)
	ipResp.Body.Close()
	outboundIP := strings.TrimSpace(string(ipBytes))

	hyperSession := hyper.NewSession(s.cfg.Akamai.APIKey)
	sbsdPayload, err := hyperSession.GenerateSbsdData(ctx, &hyper.SbsdInput{
		Index:          0,
		UserAgent:      userAgent,
		Uuid:           vValue,
		PageUrl:        baseURL + "/",
		OCookie:        oCookie,
		Script:         sbsdScript,
		AcceptLanguage: acceptLang,
		IP:             outboundIP,
	})
	if err != nil {
		return nil, fmt.Errorf("generate SBSD payload: %w", err)
	}
	sbsdBodyBytes, _ := json.Marshal(map[string]string{"body": sbsdPayload})
	sbsdPostURL, _, _ := strings.Cut(sbsdURL, "?")
	sbsdPostReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sbsdPostURL, strings.NewReader(string(sbsdBodyBytes)))
	if err != nil {
		return nil, err
	}
	sbsdPostReq.Header = http.Header{
		"x-dtreferer":        {baseURL + "/"},
		"sec-ch-ua-platform": {`"Windows"`},
		"user-agent":         {userAgent},
		"x-dtpc":             {generateXDTPCCookie()},
		"sec-ch-ua":          {secChUA},
		"content-type":       {"application/json"},
		"sec-ch-ua-mobile":   {"?0"},
		"accept":             {"*/*"},
		"origin":             {baseURL},
		"sec-fetch-site":     {"same-origin"},
		"sec-fetch-mode":     {"cors"},
		"sec-fetch-dest":     {"empty"},
		"referer":            {baseURL + "/"},
		"accept-encoding":    {"gzip, deflate, br, zstd"},
		"accept-language":    {acceptLang},
		"priority":           {"u=1, i"},
	}
	sbsdPostResp, err := client.Do(sbsdPostReq)
	if err != nil {
		return nil, fmt.Errorf("post SBSD: %w", err)
	}
	_, _ = io.ReadAll(sbsdPostResp.Body)
	sbsdPostResp.Body.Close()
	if sbsdPostResp.StatusCode != http.StatusOK && sbsdPostResp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("SBSD POST failed with status %d", sbsdPostResp.StatusCode)
	}

	if v3URL == "" {
		homeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
		if err != nil {
			return nil, err
		}
		homeReq.Header = req.Header.Clone()
		homeResp, err := client.Do(homeReq)
		if err != nil {
			return nil, err
		}
		homeBytes, _ := io.ReadAll(homeResp.Body)
		homeResp.Body.Close()
		sbsdPath, _, _ := strings.Cut(strings.TrimPrefix(sbsdURL, baseURL), "?")
		firstSeg, _, _ := strings.Cut(strings.TrimPrefix(sbsdPath, "/"), "/")
		for _, match := range allSrcRe.FindAllStringSubmatch(string(homeBytes), -1) {
			src := match[1]
			if strings.HasPrefix(src, "/"+firstSeg+"/") && !strings.Contains(src, "?v=") {
				v3URL = baseURL + src
				break
			}
		}
		if v3URL == "" {
			return nil, fmt.Errorf("V3 script not found")
		}
	}

	v3ScriptReq, err := http.NewRequestWithContext(ctx, http.MethodGet, v3URL, nil)
	if err != nil {
		return nil, err
	}
	v3ScriptReq.Header = scriptReq.Header.Clone()
	v3ScriptReq.Header.Set("referer", baseURL+"/")
	v3ScriptResp, err := client.Do(v3ScriptReq)
	if err != nil {
		return nil, err
	}
	v3ScriptBytes, _ := io.ReadAll(v3ScriptResp.Body)
	v3ScriptResp.Body.Close()
	v3Script := string(v3ScriptBytes)
	bmSz := cookieValue(jar, baseURL, "bm_sz")
	abck := cookieValue(jar, baseURL, "_abck")
	var sensorContext string
	validCookie := false

	for index := 0; index < 7; index++ {
		input := &hyper.SensorInput{
			Abck:           abck,
			Bmsz:           bmSz,
			Version:        "3",
			PageUrl:        baseURL + "/",
			UserAgent:      userAgent,
			ScriptUrl:      v3URL,
			AcceptLanguage: acceptLang,
			IP:             outboundIP,
			Context:        sensorContext,
		}
		if index == 0 {
			input.Script = v3Script
		}
		var payload, newContext string
		for attempt := 0; attempt < 3; attempt++ {
			payload, newContext, err = hyperSession.GenerateSensorData(ctx, input)
			if err == nil {
				break
			}
			if attempt == 2 {
				return nil, fmt.Errorf("generate V3 payload: %w", err)
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		sensorContext = newContext
		v3BodyBytes, _ := json.Marshal(map[string]string{"sensor_data": payload})
		v3PostReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v3URL, strings.NewReader(string(v3BodyBytes)))
		if err != nil {
			return nil, err
		}
		v3PostReq.Header = http.Header{
			"sec-ch-ua-platform": {`"Windows"`},
			"user-agent":         {userAgent},
			"x-dtpc":             {generateXDTPCCookie()},
			"sec-ch-ua":          {secChUA},
			"content-type":       {"text/plain;charset=UTF-8"},
			"sec-ch-ua-mobile":   {"?0"},
			"accept":             {"*/*"},
			"origin":             {baseURL},
			"sec-fetch-site":     {"same-origin"},
			"sec-fetch-mode":     {"cors"},
			"sec-fetch-dest":     {"empty"},
			"referer":            {baseURL + "/"},
			"accept-encoding":    {"gzip, deflate, br, zstd"},
			"accept-language":    {acceptLang},
			"priority":           {"u=1, i"},
		}
		v3PostResp, err := client.Do(v3PostReq)
		if err != nil {
			return nil, fmt.Errorf("post V3 sensor: %w", err)
		}
		_, _ = io.ReadAll(v3PostResp.Body)
		v3PostResp.Body.Close()
		abck = cookieValue(jar, baseURL, "_abck")
		if strings.Contains(abck, "~0~") {
			validCookie = true
			break
		}
	}
	if !validCookie {
		return nil, fmt.Errorf("failed to obtain valid Akamai cookie")
	}
	return &AkamaiClient{Client: client, Jar: jar, CreatedAt: time.Now().UTC()}, nil
}

func (s *AkamaiService) requestTimeoutSeconds() int {
	if s.cfg.Proxy.TimeoutSecs > 0 {
		return s.cfg.Proxy.TimeoutSecs
	}
	return 30
}

func cookieValue(jar *cookiejar.Jar, baseURL, name string) string {
	parsedURL, _ := url.Parse(baseURL)
	for _, cookie := range jar.Cookies(parsedURL) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func generateXDTPCCookie() string {
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	value := 0
	for value == 0 {
		value = random.Intn(199999) - 99999
	}
	second := random.Intn(900000000) + 100000000
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	third := make([]byte, 35)
	for index := range third {
		third[index] = alpha[random.Intn(len(alpha))]
	}
	const hexAlphabet = "0123456789abcdef"
	fourth := make([]byte, 3)
	for index := range fourth {
		fourth[index] = hexAlphabet[random.Intn(len(hexAlphabet))]
	}
	return fmt.Sprintf("%d$%d_%s-%s", value, second, string(third), string(fourth))
}
