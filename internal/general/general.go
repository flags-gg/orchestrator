package general

import (
	"net/http"

	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) KeycloakEvents(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)
}

func (s *System) StripeEvents(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)

	//const MaxBodyBytes = int64(65536)
	//r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	//payload, err := io.ReadAll(r.Body)
	//if err != nil {
	//	logs.Logf("Error reading request body: %v\n", err)
	//	w.WriteHeader(http.StatusServiceUnavailable)
	//	return
	//}
	//
	//// This is your Stripe CLI webhook secret for testing your endpoint locally.
	//endpointSecret := s.Config.Local.GetValue("STRIPE_LOCAL")
	//// Pass the request body and Stripe-Signature header to ConstructEvent, along
	//// with the webhook signing key.
	//event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), endpointSecret)
	//
	//if err != nil {
	//	logs.Logf("Error verifying webhook signature: %v\n", err)
	//	w.WriteHeader(http.StatusBadRequest) // Return a 400 error on a bad signature
	//	return
	//}
	//
	//// Unmarshal the event data into an appropriate struct depending on its Type
	//logs.Logf("Unhandled event type: %s\n", event.Type)
	//
	//w.WriteHeader(http.StatusOK)
}
