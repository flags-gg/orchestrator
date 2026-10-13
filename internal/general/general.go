package general

import (
	"io"
	"net/http"

	"github.com/flags-gg/orchestrator/internal/company"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stripe/stripe-go/webhook"
)

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

const maxStripeBodyBytes = int64(65536)

func (s *System) StripeEvents(w http.ResponseWriter, r *http.Request) {
	// Without a signing secret nothing can be verified, so nothing is accepted
	secret, _ := s.Config.ProjectProperties["stripe_webhook_secret"].(string)
	if secret == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("STRIPE_WEBHOOK_SECRET is not set, rejecting webhook")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStripeBodyBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), secret)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := company.NewSystem(s.Config).HandleStripeEvent(r.Context(), event); err != nil {
		// non-2xx makes Stripe retry
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
