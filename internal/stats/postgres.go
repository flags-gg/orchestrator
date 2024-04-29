package stats

func (s *System) GetNamesForData(data *AgentStat) (*AgentStat, error) {
	agentName, err := s.GetAgentName(data.ID)
	if err != nil {
		return data, err
	}
	data.Name = agentName

	for j, env := range data.Environments {
		environmentName, err := s.GetEnvironmentName(env.Id)
		if err != nil {
			return data, err
		}

		data.Environments[j].Name = environmentName
	}

	return data, nil
}

func (s *System) GetAgentName(agentId string) (string, error) {
	return "bob", nil
}

func (s *System) GetEnvironmentName(environmentId string) (string, error) {
	return "bill", nil
}
