package stats

import (
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"time"
)

type Environment struct {
	Id   string `json:"id"`
	Name string `json:"name"`

	Stats []Stat `json:"stats"`
}

type Stat struct {
	Requests  int64  `json:"request"`
	Errors    int64  `json:"error"`
	Successes int64  `json:"success"`
	Label     string `json:"label"`
}

type AgentStat struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Stats        []Stat        `json:"stats"`
	Environments []Environment `json:"environments"`
}

type AgentsStats struct {
	Agents []AgentStat `json:"agents"`
}

func (s *System) AddAgentSuccess(companyId, agentId, environmentId string) {
	client := influxdb2.NewClient(s.Config.Influx.Host, s.Config.Influx.Token)
	writeClient := client.WriteAPI(s.Config.Influx.Org, s.Config.Influx.Bucket)

	if environmentId == "" {
		environmentId = "dev"
	}

	p := influxdb2.NewPoint("agent",
		map[string]string{
			"company_id":     companyId,
			"agent_id":       agentId,
			"environment_id": environmentId,
		},
		map[string]interface{}{
			"request": 1,
			"error":   0,
			"success": 1,
		},
		time.Now())

	writeClient.WritePoint(p)
	writeClient.Flush()
}

func (s *System) AddAgentError(companyId, agentId, environmentId string) {
	client := influxdb2.NewClient(s.Config.Influx.Host, s.Config.Influx.Token)
	writeClient := client.WriteAPI(s.Config.Influx.Org, s.Config.Influx.Bucket)

	if environmentId == "" {
		environmentId = "dev"
	}

	p := influxdb2.NewPoint("agent",
		map[string]string{
			"company_id":     companyId,
			"agent_id":       agentId,
			"environment_id": environmentId,
		},
		map[string]interface{}{
			"request": 1,
			"success": 0,
			"error":   1,
		},
		time.Now())

	writeClient.WritePoint(p)
	writeClient.Flush()
}

func (s *System) GetAgentEnvironmentStats(agentId string, timePeriod int) (*AgentStat, error) {
	client := influxdb2.NewClient(s.Config.Influx.Host, s.Config.Influx.Token)
	queryAPI := client.QueryAPI(s.Config.Influx.Org)

	query := fmt.Sprintf(`from(bucket: "%s")
    |> range(start: -%dd)
    |> filter(fn: (r) => r._measurement == "agent" and r.agent_id == "%s")
    |> filter(fn: (r) => r._field == "error" or r._field == "request" or r._field == "success")
    |> truncateTimeColumn(unit: 1d)
    |> group(columns: ["agent_id", "environment_id", "_field"])
    |> aggregateWindow(every: 1d, fn: sum, createEmpty: false)
    |> yield(name: "dailyCounts")`, s.Config.Influx.Bucket, timePeriod, agentId)
	result, err := queryAPI.Query(s.Context, query)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query influx: %v", err)
	}

	defer func() {
		if err := result.Close(); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close query result: %v", err)
		}
	}()

	agentStat := &AgentStat{
		ID:           agentId,
		Stats:        make([]Stat, 0),
		Environments: make([]Environment, 0),
	}

	envStatsMap := make(map[string]map[string]*Stat)

	for result.Next() {
		if result.TableChanged() {
			continue
		}

		values := result.Record().Values()
		field := values["_field"].(string)
		count := values["_value"].(int64)
		environment := values["environment_id"].(string)
		agentTime := values["_time"].(time.Time).Truncate(24 * time.Hour).Format("2006-01-02") // Ensures time is truncated to start of day

		// Initialize map for new environment
		if _, exists := envStatsMap[environment]; !exists {
			envStatsMap[environment] = make(map[string]*Stat)
		}
		if _, exists := envStatsMap[environment][agentTime]; !exists {
			envStatsMap[environment][agentTime] = &Stat{Label: agentTime}
		}

		stat := envStatsMap[environment][agentTime]
		switch field {
		case "request":
			stat.Requests = count
		case "error":
			stat.Errors = count
		case "success":
			stat.Successes = count
		}
	}

	if result.Err() != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent stats from influx: %v", result.Err())
	}

	totalStats := make(map[string]*Stat)
	for envId, dailyStats := range envStatsMap {
		env := Environment{
			Id:    envId,
			Name:  fmt.Sprintf("Environment %s", envId),
			Stats: make([]Stat, 0, len(dailyStats)),
		}
		for _, stat := range dailyStats {
			env.Stats = append(env.Stats, *stat)
			if _, exists := totalStats[stat.Label]; !exists {
				totalStats[stat.Label] = &Stat{Label: stat.Label}
			}
			totalStats[stat.Label].Requests += stat.Requests
			totalStats[stat.Label].Errors += stat.Errors
			totalStats[stat.Label].Successes += stat.Successes
		}
		agentStat.Environments = append(agentStat.Environments, env)
	}

	for _, stat := range totalStats {
		agentStat.Stats = append(agentStat.Stats, *stat)
	}

	return agentStat, nil
}

func (s *System) GetAgentsStatsFromInflux(companyId string) (*AgentsStats, error) {
	client := influxdb2.NewClient(s.Config.Influx.Host, s.Config.Influx.Token)
	queryAPI := client.QueryAPI(s.Config.Influx.Org)

	query := fmt.Sprintf(`from(bucket:"%s")|> range(start: -1d)|> filter(fn: (r) => r._measurement == "agent" and r.company_id == "%s")`, s.Config.Influx.Bucket, companyId)
	result, err := queryAPI.Query(s.Context, query)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := result.Close(); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close query result: %v", err)
		}
	}()

	agentsStats := &AgentsStats{}

	for result.Next() {
		if result.TableChanged() {
			continue
		}

		agentStat := &AgentStat{
			ID: result.Record().ValueByKey("agent_id").(string),
		}

		for result.Next() {
			if result.TableChanged() {
				continue
			}

			//agentStat.Stats = append(agentStat.Stats, Stat{
			//	Requests:  int(result.Record().ValueByKey("request").(float64)),
			//	Errors:    int(result.Record().ValueByKey("error").(float64)),
			//	Successes: int(result.Record().ValueByKey("success").(float64)),
			//	Label:     result.Record().Time().String(),
			//})
		}

		agentsStats.Agents = append(agentsStats.Agents, *agentStat)
	}

	return agentsStats, nil
}
