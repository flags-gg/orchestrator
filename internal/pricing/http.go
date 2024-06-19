package pricing

import (
	"context"
	"encoding/json"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}

func (s *System) GetCompanyPricing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type Pricing struct {
		Pricing []Price `json:"prices"`
	}
	pricing := Pricing{}

	pricing.Pricing = append(pricing.Pricing, s.GetFree())
	pricing.Pricing = append(pricing.Pricing, s.GetStartup())
	pricing.Pricing = append(pricing.Pricing, s.GetPro())
	pricing.Pricing = append(pricing.Pricing, s.GetEnterprise())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&pricing); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) GetGeneralPricing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	type Pricing struct {
		Pricing []Price `json:"prices"`
	}
	pricing := Pricing{}

	pricing.Pricing = append(pricing.Pricing, s.GetFree())
	pricing.Pricing = append(pricing.Pricing, s.GetStartup())
	pricing.Pricing = append(pricing.Pricing, s.GetPro())
	pricing.Pricing = append(pricing.Pricing, s.GetEnterprise())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&pricing); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
