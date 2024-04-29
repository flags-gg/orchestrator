package flags

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/config"
	"github.com/flags-gg/orchestrator/internal/stats"
	"net/http"
	"strconv"
	"time"
)

type SecretMenuStyle struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type SecretMenu struct {
	Sequence []string          `json:"sequence,omitempty"`
	Styles   []SecretMenuStyle `json:"styles,omitempty"`
}
type Details struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
type Flag struct {
	Enabled bool    `json:"enabled"`
	Details Details `json:"details"`
}
type Response struct {
	IntervalAllowed int        `json:"intervalAllowed,omitempty"`
	SecretMenu      SecretMenu `json:"secretMenu,omitempty"`
	Flags           []Flag     `json:"flags,omitempty"`
}

type System struct {
	Config  *config.Config
	Context context.Context
}

func NewFlagsSystem(cfg *config.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) GetFlags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-company-id") == "" || r.Header.Get("x-agent-id") == "" {
		res := Response{
			IntervalAllowed: 600,
			Flags:           []Flag{},
		}
		if err := json.NewEncoder(w).Encode(res); err != nil {
			_ = logs.Local().Errorf("Failed to encode response: %v", err)
		}
	}

	// get the flags
	res := Response{
		IntervalAllowed: 60,
		SecretMenu: SecretMenu{
			//Sequence: []string{"ArrowUp", "ArrowUp", "ArrowDown", "ArrowDown", "ArrowLeft", "ArrowRight", "ArrowLeft", "ArrowRight", "b", "a"},
			Sequence: []string{"ArrowDown", "ArrowDown", "ArrowDown", "b", "b"},
			//Styles: []SecretMenuStyle{
			//	{
			//		Name:  "closeButton",
			//		Value: `position: "absolute"; top: "0px"; right: "0px"; background: "white"; color: "purple"; cursor: "pointer";`,
			//	},
			//	{
			//		Name:  "container",
			//		Value: `position: "fixed"; top: "50%"; left: "50%"; transform: "translate(-50%, -50%)"; zIndex: 1000; backgroundColor: "white"; color: "black"; border: "1px solid black"; borderRadius: "5px"; padding: "1rem";`,
			//	},
			//	{
			//		Name:  "button",
			//		Value: `display: "flex"; justifyContent: "space-between"; alignItems: "center"; padding: "0.5rem"; background: "lightgray"; borderRadius: "5px"; margin: "0.5rem 0";`,
			//	},
			//},
		},
		Flags: []Flag{
			{
				Enabled: true,
				Details: Details{
					Name: "perAgent",
					ID:   "1",
				},
			},
			{
				Enabled: true,
				Details: Details{
					Name: "totalRequests",
					ID:   "2",
				},
			},
			{
				Enabled: true,
				Details: Details{
					Name: "notifications",
					ID:   "3",
				},
			},
		},
	}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		_ = logs.Local().Errorf("Failed to encode response: %v", err)
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		stats.NewStatsSystem(s.Config).AddAgentError(r.Header.Get("x-company-id"), r.Header.Get("x-agent-id"), r.Header.Get("x-environment-id"))
		return
	}
	stats.NewStatsSystem(s.Config).AddAgentSuccess(r.Header.Get("x-company-id"), r.Header.Get("x-agent-id"), r.Header.Get("x-environment-id"))
}

func CreateFlags(w http.ResponseWriter, r *http.Request) {

}

func UpdateFlags(w http.ResponseWriter, r *http.Request) {

}

func DeleteFlags(w http.ResponseWriter, r *http.Request) {

}
