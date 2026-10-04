package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/paystack"
	"github.com/var-raphael/groundtruth/internal/plans"
)

const planCacheTTL = 10 * time.Minute

type BillingHandler struct {
	Pool        *pgxpool.Pool
	Paystack    *paystack.Client
	ProPlanCode string
	AppURL      string

	mu          sync.Mutex
	cachedPlan  *paystack.Plan
	planFetched time.Time
}

func (h *BillingHandler) configured() bool {
	return h.Paystack.Configured() && h.ProPlanCode != ""
}

func (h *BillingHandler) proPlan(r *http.Request) (*paystack.Plan, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cachedPlan != nil && time.Since(h.planFetched) < planCacheTTL {
		return h.cachedPlan, nil
	}
	plan, err := h.Paystack.FetchPlan(r.Context(), h.ProPlanCode)
	if err != nil {
		return nil, err
	}
	h.cachedPlan = plan
	h.planFetched = time.Now()
	return plan, nil
}

func (h *BillingHandler) Pricing(w http.ResponseWriter, r *http.Request) {
	display := map[string]any{
		"amount":   plans.For(plans.Pro).PriceUSD * 100,
		"currency": "USD",
		"interval": "monthly",
	}

	if !h.configured() {
		writeJSON(w, http.StatusOK, map[string]any{"pro": display, "display": display})
		return
	}

	plan, err := h.proPlan(r)
	if err != nil {
		log.Printf("billing: fetching plan for pricing: %v", err)
		writeJSON(w, http.StatusOK, map[string]any{"pro": display, "display": display})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pro":     map[string]any{"amount": plan.Amount, "currency": plan.Currency, "interval": plan.Interval},
		"display": display,
	})
}

func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "payments are not configured yet"})
		return
	}

	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return
	}

	billing, err := queries.GetBilling(r.Context(), h.Pool, recruiterID)
	if err != nil || billing == nil {
		http.Error(w, "could not load your account", http.StatusInternalServerError)
		return
	}
	if billing.Plan != plans.Free {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "you are already on a paid plan"})
		return
	}

	plan, err := h.proPlan(r)
	if err != nil {
		log.Printf("billing: fetching plan: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach the payment provider"})
		return
	}

	callback := strings.TrimRight(h.AppURL, "/") + "/pricing?checkout=done"
	url, err := h.Paystack.InitializeTransaction(r.Context(), billing.Email, plan.Amount, h.ProPlanCode, callback,
		map[string]string{"recruiter_id": recruiterID})
	if err != nil {
		log.Printf("billing: initializing transaction: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not start checkout"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (h *BillingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "payments are not configured yet"})
		return
	}

	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return
	}

	billing, err := queries.GetBilling(r.Context(), h.Pool, recruiterID)
	if err != nil || billing == nil {
		http.Error(w, "could not load your account", http.StatusInternalServerError)
		return
	}
	if billing.SubscriptionCode == "" || billing.EmailToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no active subscription to cancel"})
		return
	}

	if err := h.Paystack.DisableSubscription(r.Context(), billing.SubscriptionCode, billing.EmailToken); err != nil {
		log.Printf("billing: cancelling subscription: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not cancel, please try again"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type paystackEvent struct {
	Event string `json:"event"`
	Data  struct {
		Metadata         json.RawMessage `json:"metadata"`
		Customer         json.RawMessage `json:"customer"`
		Plan             json.RawMessage `json:"plan"`
		Authorization    json.RawMessage `json:"authorization"`
		SubscriptionCode string          `json:"subscription_code"`
		EmailToken       string          `json:"email_token"`
		NextPaymentDate  string          `json:"next_payment_date"`
	} `json:"data"`
}

func planCodeOf(raw json.RawMessage) string {
	var p struct {
		PlanCode string `json:"plan_code"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	return p.PlanCode
}

func customerOf(raw json.RawMessage) (code, email string) {
	var c struct {
		CustomerCode string `json:"customer_code"`
		Email        string `json:"email"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return "", ""
	}
	return c.CustomerCode, c.Email
}

func metaRecruiterID(raw json.RawMessage) string {
	var m struct {
		RecruiterID string `json:"recruiter_id"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return m.RecruiterID
}

func cardBrandOf(raw json.RawMessage) string {
	var a struct {
		Brand    string `json:"brand"`
		CardType string `json:"card_type"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return "unknown"
	}
	brand := strings.ToLower(strings.TrimSpace(a.Brand))
	if brand == "" {
		brand = strings.ToLower(strings.TrimSpace(a.CardType))
	}
	if brand == "" {
		return "unknown"
	}
	return brand
}

func (h *BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}

	if !h.Paystack.VerifySignature(body, r.Header.Get("x-paystack-signature")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	seen, err := queries.PaymentEventSeen(r.Context(), h.Pool, hash)
	if err != nil {
		log.Printf("billing webhook: %v", err)
		http.Error(w, "try again", http.StatusInternalServerError)
		return
	}
	if seen {
		w.WriteHeader(http.StatusOK)
		return
	}

	var event paystackEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	if err := h.handleEvent(r, event); err != nil {
		log.Printf("billing webhook: handling %s: %v", event.Event, err)
		http.Error(w, "try again", http.StatusInternalServerError)
		return
	}

	if err := queries.MarkPaymentEvent(r.Context(), h.Pool, hash, event.Event); err != nil {
		log.Printf("billing webhook: %v", err)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *BillingHandler) handleEvent(r *http.Request, event paystackEvent) error {
	ctx := r.Context()
	d := event.Data
	customerCode, customerEmail := customerOf(d.Customer)
	planCode := planCodeOf(d.Plan)

	switch event.Event {
	case "charge.success":
		metaID := metaRecruiterID(d.Metadata)
		if metaID == "" && (planCode == "" || planCode != h.ProPlanCode) {
			return nil
		}
		id, err := queries.FindRecruiterForPayment(ctx, h.Pool, metaID, "", customerCode, customerEmail)
		if err != nil {
			return err
		}
		if id == "" {
			log.Printf("billing webhook: charge.success for unknown customer %s", customerEmail)
			return nil
		}
		brand := cardBrandOf(d.Authorization)
		log.Printf("billing: activating pro plan for recruiter %s, card brand %s", id, brand)
		if err := queries.RecordCardBrand(ctx, h.Pool, id, brand); err != nil {
			log.Printf("billing webhook: %v", err)
		}
		return queries.ActivateProPlan(ctx, h.Pool, id, customerCode)

	case "subscription.create":
		if planCode != h.ProPlanCode {
			return nil
		}
		id, err := queries.FindRecruiterForPayment(ctx, h.Pool, "", d.SubscriptionCode, customerCode, customerEmail)
		if err != nil {
			return err
		}
		if id == "" {
			log.Printf("billing webhook: subscription.create for unknown customer %s", customerEmail)
			return nil
		}
		return queries.SaveSubscription(ctx, h.Pool, id, d.SubscriptionCode, d.EmailToken, customerCode)

	case "subscription.not_renew":
		if planCode != h.ProPlanCode {
			return nil
		}
		id, err := queries.FindRecruiterForPayment(ctx, h.Pool, "", d.SubscriptionCode, customerCode, customerEmail)
		if err != nil || id == "" {
			return err
		}
		expiresAt, parseErr := time.Parse(time.RFC3339, d.NextPaymentDate)
		if parseErr != nil {
			log.Printf("billing webhook: could not read next_payment_date %q", d.NextPaymentDate)
			return nil
		}
		log.Printf("billing: recruiter %s cancelled, pro stays until %s", id, expiresAt.Format(time.RFC3339))
		return queries.SetPlanExpiry(ctx, h.Pool, id, expiresAt)

	case "subscription.disable":
		if planCode != h.ProPlanCode {
			return nil
		}
		id, err := queries.FindRecruiterForPayment(ctx, h.Pool, "", d.SubscriptionCode, customerCode, customerEmail)
		if err != nil || id == "" {
			return err
		}
		log.Printf("billing: subscription ended, downgrading recruiter %s", id)
		return queries.DowngradeToFree(ctx, h.Pool, id)
	}

	return nil
}
