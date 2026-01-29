package pricing

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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

func (s *System) GetCompanyPricing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type Pricing struct {
		Pricing []Price `json:"prices"`
	}
	pricing := Pricing{}

	//pricing.Pricing = append(pricing.Pricing, s.GetFree())
	pricing.Pricing = append(pricing.Pricing, s.GetStartup(ctx))
	pricing.Pricing = append(pricing.Pricing, s.GetPro(ctx))
	pricing.Pricing = append(pricing.Pricing, s.GetEnterprise(ctx))

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&pricing); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) GetGeneralPricing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	type Pricing struct {
		Pricing []Price `json:"prices"`
	}
	returnedPrices, err := s.GetPrices(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	pricing := Pricing{
		Pricing: returnedPrices,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&pricing); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
