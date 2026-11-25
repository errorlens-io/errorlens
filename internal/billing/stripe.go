// Package billing integrates Stripe subscriptions for the hosted service.
// It uses Stripe's REST API directly (no SDK) and is entirely optional:
// self-hosted servers leave it unconfigured.
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Plan is a paid plan backed by a Stripe price.
type Plan struct {
	ID      string `json:"id"`    // e.g. "team"
	PriceID string `json:"-"`     // Stripe price id
	Quota   int64  `json:"quota"` // monthly events
	Label   string `json:"label"` // display name
}

// Stripe holds API credentials and the plan catalog.
type Stripe struct {
	SecretKey     string
	WebhookSecret string
	Plans         []Plan
	HTTP          *http.Client
	APIBase       string // overridable in tests
}

// FromEnv reads ROOTCAUSE_STRIPE_SECRET_KEY, ROOTCAUSE_STRIPE_WEBHOOK_SECRET
// and ROOTCAUSE_STRIPE_PLANS ("team:price_123:100000,business:price_456:1000000").
// It returns nil when billing is not configured.
func FromEnv(getenv func(string) string) (*Stripe, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	key := getenv("ROOTCAUSE_STRIPE_SECRET_KEY")
	if key == "" {
		return nil, nil
	}
	s := &Stripe{SecretKey: key, WebhookSecret: getenv("ROOTCAUSE_STRIPE_WEBHOOK_SECRET"),
		HTTP: &http.Client{Timeout: 20 * time.Second}, APIBase: "https://api.stripe.com"}
	if s.WebhookSecret == "" {
		return nil, errors.New("ROOTCAUSE_STRIPE_WEBHOOK_SECRET is required when Stripe is enabled")
	}
	for _, spec := range strings.Split(getenv("ROOTCAUSE_STRIPE_PLANS"), ",") {
		parts := strings.Split(strings.TrimSpace(spec), ":")
		if len(parts) != 3 {
			continue
		}
		q, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ROOTCAUSE_STRIPE_PLANS: bad quota in %q", spec)
		}
		s.Plans = append(s.Plans, Plan{ID: parts[0], PriceID: parts[1], Quota: q, Label: strings.ToUpper(parts[0][:1]) + parts[0][1:]})
	}
	if len(s.Plans) == 0 {
		return nil, errors.New("ROOTCAUSE_STRIPE_PLANS must list at least one plan as id:price_id:monthly_events")
	}
	return s, nil
}

// PlanByID finds a plan.
func (s *Stripe) PlanByID(id string) (Plan, bool) {
	for _, p := range s.Plans {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}

// PlanByPrice finds a plan by Stripe price id.
func (s *Stripe) PlanByPrice(price string) (Plan, bool) {
	for _, p := range s.Plans {
		if p.PriceID == price {
			return p, true
		}
	}
	return Plan{}, false
}

func (s *Stripe) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.APIBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("stripe: %s (%d)", e.Error.Message, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// CheckoutURL creates a subscription checkout session for an organization.
func (s *Stripe) CheckoutURL(ctx context.Context, orgID, customerID, email string, plan Plan, successURL, cancelURL string) (string, error) {
	form := url.Values{
		"mode":                                {"subscription"},
		"line_items[0][price]":                {plan.PriceID},
		"line_items[0][quantity]":             {"1"},
		"client_reference_id":                 {orgID},
		"success_url":                         {successURL},
		"cancel_url":                          {cancelURL},
		"metadata[org_id]":                    {orgID},
		"subscription_data[metadata][org_id]": {orgID},
	}
	if customerID != "" {
		form.Set("customer", customerID)
	} else if email != "" {
		form.Set("customer_email", email)
	}
	var out struct{ URL string }
	if err := s.post(ctx, "/v1/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// PortalURL opens Stripe's customer portal (manage card, cancel, invoices).
func (s *Stripe) PortalURL(ctx context.Context, customerID, returnURL string) (string, error) {
	var out struct{ URL string }
	err := s.post(ctx, "/v1/billing_portal/sessions", url.Values{"customer": {customerID}, "return_url": {returnURL}}, &out)
	return out.URL, err
}

// Event is the subset of a Stripe webhook event we use.
type Event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// VerifyWebhook checks the Stripe-Signature header (t=...,v1=...) and
// rejects events older than 5 minutes to stop replays.
func (s *Stripe) VerifyWebhook(payload []byte, header string, now time.Time) (*Event, error) {
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return nil, errors.New("missing signature")
	}
	if d := now.Sub(time.Unix(ts, 0)); d > 5*time.Minute || d < -5*time.Minute {
		return nil, errors.New("signature timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(payload)
	want := mac.Sum(nil)
	valid := false
	for _, sig := range sigs {
		if b, err := hex.DecodeString(sig); err == nil && hmac.Equal(b, want) {
			valid = true
		}
	}
	if !valid {
		return nil, errors.New("invalid signature")
	}
	var ev Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// Sign produces a Stripe-Signature header value (used by tests).
func Sign(secret string, payload []byte, t time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", t.Unix())
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", t.Unix(), hex.EncodeToString(mac.Sum(nil)))
}
