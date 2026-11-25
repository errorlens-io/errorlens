package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/audit"
	"github.com/errorlens-io/errorlens/internal/billing"
)

// WithBilling enables Stripe billing endpoints.
func (s *Server) WithBilling(b *billing.Stripe) *Server { s.billing = b; return s }

// --- usage & billing (org owners) ---------------------------------------------------------

type usageDay struct {
	Day      string `json:"day"`
	Accepted int64  `json:"accepted"`
	Dropped  int64  `json:"dropped"`
	Logs     int64  `json:"logs"`
}

func (s *Server) getBilling(w http.ResponseWriter, r *http.Request) {
	var plan, status, customer string
	var quota *int64
	var days []usageDay
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `SELECT plan, monthly_event_quota, billing_status, billing_customer_id FROM organizations WHERE id = app_org_id()`).
			Scan(&plan, &quota, &status, &customer); err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT to_char(day, 'YYYY-MM-DD'), events_accepted, events_dropped, logs_accepted
			FROM usage_daily WHERE day >= date_trunc('month', now() AT TIME ZONE 'UTC')::date ORDER BY day`)
		if err != nil {
			return err
		}
		days, err = pgx.CollectRows(rows, pgx.RowToStructByPos[usageDay])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var used, dropped int64
	for _, d := range days {
		used += d.Accepted
		dropped += d.Dropped
	}
	if days == nil {
		days = []usageDay{}
	}
	resp := map[string]any{
		"plan": plan, "quota": quota, "used": used, "dropped": dropped, "days": days,
		"billingEnabled": s.billing != nil, "status": status, "hasCustomer": customer != "",
	}
	if s.billing != nil {
		resp["plans"] = s.billing.Plans
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeError(w, http.StatusNotFound, "billing is not enabled on this server")
		return
	}
	var req struct {
		Plan string `json:"plan"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	plan, ok := s.billing.PlanByID(req.Plan)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown plan")
		return
	}
	org := orgFrom(r.Context())
	var customer string
	_ = s.db.Pool.QueryRow(r.Context(), `SELECT billing_customer_id FROM organizations WHERE id = $1`, org.ID).Scan(&customer)
	base := s.cfg.PublicURL.String() + "/" + org.Slug + "/settings/billing"
	u, err := s.billing.CheckoutURL(r.Context(), org.ID.String(), customer, userFrom(r.Context()).Email, plan, base+"?checkout=success", base)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

func (s *Server) billingPortal(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeError(w, http.StatusNotFound, "billing is not enabled on this server")
		return
	}
	org := orgFrom(r.Context())
	var customer string
	_ = s.db.Pool.QueryRow(r.Context(), `SELECT billing_customer_id FROM organizations WHERE id = $1`, org.ID).Scan(&customer)
	if customer == "" {
		writeError(w, http.StatusBadRequest, "no subscription yet")
		return
	}
	u, err := s.billing.PortalURL(r.Context(), customer, s.cfg.PublicURL.String()+"/"+org.Slug+"/settings/billing")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

// stripeWebhook applies subscription changes. Mounted outside the session and
// CSRF middleware; authenticity comes from the signature.
func (s *Server) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	ev, err := s.billing.VerifyWebhook(body, r.Header.Get("Stripe-Signature"), time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	type subscription struct {
		ID       string            `json:"id"`
		Customer string            `json:"customer"`
		Status   string            `json:"status"`
		Metadata map[string]string `json:"metadata"`
		Items    struct {
			Data []struct {
				Price struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	}
	ctx := r.Context()
	apply := func(orgID string, set map[string]any) error {
		id, err := uuid.Parse(orgID)
		if err != nil {
			return nil // not ours
		}
		_, err = s.db.Pool.Exec(ctx, `UPDATE organizations SET
			plan = COALESCE($2, plan), monthly_event_quota = CASE WHEN $3::boolean THEN $4 ELSE monthly_event_quota END,
			billing_customer_id = COALESCE($5, billing_customer_id), billing_status = COALESCE($6, billing_status)
			WHERE id = $1`, id, set["plan"], set["quota"] != nil, set["quota"], set["customer"], set["status"])
		return err
	}
	switch ev.Type {
	case "checkout.session.completed":
		var cs struct {
			ClientReferenceID string `json:"client_reference_id"`
			Customer          string `json:"customer"`
		}
		_ = json.Unmarshal(ev.Data.Object, &cs)
		err = apply(cs.ClientReferenceID, map[string]any{"customer": cs.Customer, "status": "active"})
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub subscription
		_ = json.Unmarshal(ev.Data.Object, &sub)
		set := map[string]any{"customer": sub.Customer, "status": sub.Status}
		if ev.Type == "customer.subscription.deleted" || sub.Status == "canceled" || sub.Status == "unpaid" {
			set["plan"], set["quota"] = "free", s.cfg.FreeEventQuota
		} else if len(sub.Items.Data) > 0 {
			if p, ok := s.billing.PlanByPrice(sub.Items.Data[0].Price.ID); ok {
				set["plan"], set["quota"] = p.ID, p.Quota
			}
		}
		err = apply(sub.Metadata["org_id"], set)
	}
	if err != nil {
		s.log.Error("stripe webhook", "type", ev.Type, "err", err)
		http.Error(w, "retry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- operator console (superusers) --------------------------------------------------------

func requireSuperuser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r.Context()); u == nil || !u.IsSuperuser || r.Context().Value(tokenKey) != nil {
			writeError(w, http.StatusForbidden, "instance administrators only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type adminOrg struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`
	Quota     *int64    `json:"quota"`
	Status    string    `json:"billingStatus"`
	Members   int64     `json:"members"`
	Projects  int64     `json:"projects"`
	UsedMonth int64     `json:"usedThisMonth"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) adminListOrgs(w http.ResponseWriter, r *http.Request) {
	var out []adminOrg
	// Cross-tenant by design: only instance administrators reach this.
	err := s.db.System(r.Context(), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT o.id, o.name, o.slug, o.plan, o.monthly_event_quota, o.billing_status,
			       (SELECT count(*) FROM memberships m WHERE m.org_id = o.id),
			       (SELECT count(*) FROM projects p WHERE p.org_id = o.id),
			       COALESCE((SELECT sum(events_accepted) FROM usage_daily u WHERE u.org_id = o.id
			                 AND u.day >= date_trunc('month', now() AT TIME ZONE 'UTC')::date), 0)::bigint,
			       o.created_at
			FROM organizations o ORDER BY o.created_at DESC LIMIT 500`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[adminOrg])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []adminOrg{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminUpdateOrg(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req struct {
		Plan  string `json:"plan"`
		Quota *int64 `json:"quota"` // null = unlimited
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Plan == "" || len(req.Plan) > 50 || (req.Quota != nil && *req.Quota < 0) {
		writeError(w, http.StatusBadRequest, "plan is required and quota must be ≥ 0 or null")
		return
	}
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE organizations SET plan = $2, monthly_event_quota = $3 WHERE id = $1`, id, req.Plan, req.Quota)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: id, ActorID: userFrom(ctx).ID, Action: "admin.plan_change", Target: req.Plan, IP: clientIP(r)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	s.finish(w, r, err, map[string]bool{"ok": true})
}
