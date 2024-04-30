package stats

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/config"
	"net/http"
	"strconv"
	"time"
)

type System struct {
	Config  *config.Config
	Context context.Context
}

func NewStatsSystem(cfg *config.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) GetEnvironmentStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&AgentStat{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	agentId := r.PathValue("agentId")
	if agentId == "" {
		if err := json.NewEncoder(w).Encode(&AgentStat{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	s.Context = r.Context()
	timePeriod := 30
	if r.URL.Query().Get("timePeriod") != "" {
		timePeriod, _ = strconv.Atoi(r.URL.Query().Get("timePeriod"))
	}

	data, err := s.GetAgentEnvironmentStats(agentId, timePeriod)
	if err != nil {
		_ = logs.Errorf("Failed to get agent stats from influx: %v", err)
		//w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data, err = s.GetNamesForData(data)
	if err != nil {
		_ = logs.Errorf("Failed to get names for agent stats: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) GetAgentStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&AgentStat{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	agentId := r.PathValue("agentId")
	if agentId == "" {
		if err := json.NewEncoder(w).Encode(&AgentStat{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	s.Context = r.Context()
	timePeriod := 30
	if r.URL.Query().Get("timePeriod") != "" {
		timePeriod, _ = strconv.Atoi(r.URL.Query().Get("timePeriod"))
	}

	data, err := s.GetAgentEnvironmentStats(agentId, timePeriod)
	if err != nil {
		_ = logs.Errorf("Failed to get agent stats from influx: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data, err = s.GetNamesForData(data)
	if err != nil {
		_ = logs.Errorf("Failed to get names for agent stats: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) GetAgentsStats(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&AgentsStats{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	res := AgentsStats{
		Agents: []AgentStat{
			{
				ID:   "123",
				Name: "Agent 123",
				Environments: []Environment{
					{
						Id:   "123",
						Name: "Environment 123",
						Stats: []Stat{
							{
								Label:     "January",
								Requests:  123,
								Errors:    24,
								Successes: 99,
							},
							{
								Label:     "February",
								Requests:  98,
								Errors:    56,
								Successes: 42,
							},
							{
								Label:     "March",
								Requests:  150,
								Errors:    75,
								Successes: 75,
							},
							{
								Label:     "April",
								Requests:  110,
								Errors:    60,
								Successes: 50,
							},
							{
								Label:     "May",
								Requests:  120,
								Errors:    30,
								Successes: 90,
							},
							{
								Label:     "June",
								Requests:  130,
								Errors:    20,
								Successes: 110,
							},
							{
								Label:     "July",
								Requests:  140,
								Errors:    100,
								Successes: 40,
							},
							{
								Label:     "August",
								Requests:  115,
								Errors:    75,
								Successes: 40,
							},
							{
								Label:     "September",
								Requests:  125,
								Errors:    50,
								Successes: 75,
							},
							{
								Label:     "October",
								Requests:  135,
								Errors:    68,
								Successes: 67,
							},
							{
								Label:     "November",
								Requests:  145,
								Errors:    72,
								Successes: 73,
							},
							{
								Label:     "December",
								Requests:  155,
								Errors:    90,
								Successes: 65,
							},
						},
					},
				},
			},
			{
				ID:   "456",
				Name: "Bill",
				Environments: []Environment{
					{
						Id:   "456",
						Name: "Bill's Environment",
						Stats: []Stat{
							{
								Label:     "January",
								Requests:  150,
								Errors:    90,
								Successes: 60,
							},
							{
								Label:     "February",
								Requests:  120,
								Errors:    70,
								Successes: 50,
							},
							{
								Label:     "March",
								Requests:  160,
								Errors:    80,
								Successes: 80,
							},
							{
								Label:     "April",
								Requests:  100,
								Errors:    40,
								Successes: 60,
							},
							{
								Label:     "May",
								Requests:  110,
								Errors:    55,
								Successes: 55,
							},
							{
								Label:     "June",
								Requests:  140,
								Errors:    70,
								Successes: 70,
							},
							{
								Label:     "July",
								Requests:  130,
								Errors:    90,
								Successes: 40,
							},
							{
								Label:     "August",
								Requests:  125,
								Errors:    85,
								Successes: 40,
							},
							{
								Label:     "September",
								Requests:  135,
								Errors:    75,
								Successes: 60,
							},
							{
								Label:     "October",
								Requests:  145,
								Errors:    95,
								Successes: 50,
							},
							{
								Label:     "November",
								Requests:  155,
								Errors:    77,
								Successes: 78,
							},
							{
								Label:     "December",
								Requests:  165,
								Errors:    105,
								Successes: 60,
							},
						},
					},
				},
			},
		},
	}
	if err := json.NewEncoder(w).Encode(res); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
