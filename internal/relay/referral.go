package relay

import (
	"context"
	"net/http"
	"strings"
	"time"

	"queueup/internal/store"
)

// Referrals.
//
// A mate who arrives on somebody's code pays the lower first month. Once that
// mate has turned up properly, paid, and put a PC of their own on the end of
// it, the person who sent them gets a month at the same price. Three of those,
// ever, and then it stops.
//
// Both halves of "turned up properly" matter, and they are the whole defence
// against somebody referring themselves. A payment alone is cheap to fake. A
// payment plus a second Windows PC left switched on, to save three pounds, is
// a trade nobody makes.

func (s *Server) referralRoutes() {
	s.mux.HandleFunc("GET /api/referral", s.withAccount(s.handleReferral))
}

func (s *Server) handleReferral(w http.ResponseWriter, r *http.Request, acct store.Account) {
	ref, err := s.st.Referral(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't read your referrals.")
		return
	}

	// The code is minted the first time somebody looks, not at signup: most
	// accounts never share one, and every code is a Stripe object we would
	// otherwise be creating for nothing.
	if ref.Code == "" && s.stripeReady() && s.cfg.StripeIntroCouponID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		code, err := s.mintReferralCode(ctx, acct.ID)
		cancel()
		if err != nil {
			s.log.Error("minting a referral code", "account", acct.ID, "err", err)
		} else {
			ref.Code = code
		}
	}

	link := ""
	if ref.Code != "" {
		link = strings.TrimSuffix(s.cfg.WebURL, "/") + "/?promo=" + ref.Code
	}
	// Three numbers, because the rewards screen draws a slot per month and
	// each slot is in one of three states: spent, waiting, or not earned yet.
	// "How many have I got left" means both of the first two to different
	// people, so the screen shows both rather than picking one.
	writeJSON(w, http.StatusOK, map[string]any{
		"code":      ref.Code,
		"link":      link,
		"earned":    ref.Earned,
		"used":      max(0, ref.Earned-ref.Credits),
		"banked":    ref.Credits,
		"remaining": max(0, store.MaxReferralRewards-ref.Earned),
		"max":       store.MaxReferralRewards,
	})
}

// mintReferralCode creates the person's code, as a real Stripe promotion code
// against the first-month coupon.
//
// Making it a Stripe object rather than something we invent means the whole
// existing checkout path works unchanged: the code discounts the mate's first
// month, lands in the subscription's metadata, and is therefore still readable
// months later when somebody asks where this customer came from.
func (s *Server) mintReferralCode(ctx context.Context, accountID string) (string, error) {
	code := store.NewReferralCode()
	if code == "" {
		return "", errNoCode
	}
	made, err := s.cfg.Stripe.CreatePromotionCode(ctx, s.cfg.StripeIntroCouponID, code)
	if err != nil {
		return "", err
	}
	if err := s.st.SetReferralCode(accountID, made); err != nil {
		return "", err
	}
	return made, nil
}

// maybeAwardReferral pays the person who sent this account, if the account has
// now done both of the things that make it real.
//
// Called from the two places either half can land: the payment clearing, and
// the PC being paired. Whichever happens second is the one that fires.
func (s *Server) maybeAwardReferral(ctx context.Context, refereeID string) {
	ref, err := s.st.Referral(refereeID)
	if err != nil || ref.ReferredBy == "" || ref.Rewarded {
		return
	}
	sub, err := s.st.SubscriptionFor(refereeID)
	if err != nil || !sub.Active() || sub.Comped() {
		return // free access is not a sale, and nobody is owed for one
	}
	devices, err := s.st.Devices(refereeID)
	if err != nil || len(devices) == 0 {
		return
	}

	referrerID, awarded, err := s.st.AwardReferral(refereeID)
	if err != nil {
		s.log.Error("awarding a referral", "referee", refereeID, "err", err)
		return
	}
	if !awarded {
		return
	}
	s.log.Info("referral earned", "referrer", referrerID, "referee", refereeID)
	s.applyReferralCredits(ctx, referrerID)
}

// applyReferralCredits hands one banked month to Stripe, if there is one and
// the subscription is not already carrying a discount.
//
// One at a time on purpose. Stripe holds a single discount per subscription,
// and the point of the reward is a run of cheaper months rather than one
// month that happens to be free.
func (s *Server) applyReferralCredits(ctx context.Context, accountID string) {
	if accountID == "" || !s.stripeReady() || s.cfg.StripeReferralCouponID == "" {
		return
	}
	ref, err := s.st.Referral(accountID)
	if err != nil || ref.Credits <= 0 {
		return
	}
	sub, err := s.st.SubscriptionFor(accountID)
	if err != nil || sub.SubID == "" {
		return
	}
	live, err := s.cfg.Stripe.GetSubscription(ctx, sub.SubID)
	if err != nil {
		s.log.Error("reading a subscription to apply a referral month", "account", accountID, "err", err)
		return
	}
	if live.HasDiscount() {
		return // last month's reward has not been spent yet; it keeps
	}
	// Spend first. A credit taken and then failed to apply costs one person
	// one month; a credit applied and then failed to take could be applied
	// again every time the webhook fires.
	if err := s.st.SpendReferralCredit(accountID); err != nil {
		return
	}
	if err := s.cfg.Stripe.ApplyCoupon(ctx, sub.SubID, s.cfg.StripeReferralCouponID); err != nil {
		s.log.Error("applying a referral month", "account", accountID, "err", err)
		return
	}
	s.log.Info("referral month applied", "account", accountID)
}

// errNoCode means the random source failed, which is worth an error rather
// than a predictable code somebody else could guess.
var errNoCode = errorString("couldn't generate a referral code")

type errorString string

func (e errorString) Error() string { return string(e) }
