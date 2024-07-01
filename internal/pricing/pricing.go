package pricing

type Extra struct {
	Title    string `json:"title,omitempty"`
	Launched bool   `json:"launched,omitempty"`
}
type Price struct {
	Title        string  `json:"title,omitempty"`
	SubTitle     string  `json:"sub_title,omitempty"`
	Price        int     `json:"price,omitempty"`
	TeamMembers  int     `json:"team_members,omitempty"`
	Projects     int     `json:"projects,omitempty"`
	Agents       int     `json:"agents,omitempty"`
	Environments int     `json:"environments,omitempty"`
	Requests     int     `json:"requests,omitempty"`
	SupportType  string  `json:"support_type,omitempty"`
	Extras       []Extra `json:"extras,omitempty"`
}

func (s *System) GetPrices() ([]Price, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var prices []Price
	rows, err := client.Query(s.Context, `
    SELECT
      payment_plans.name,
      payment_plans.price,
      payment_plans.team_members,
      payment_plans.projects,
      payment_plans.agents,
      payment_plans.environments,
      payment_plans.requests,
      payment_plans.support_category,
      payment_plans.popular
    FROM public.payment_plans
    WHERE payment_plans.custom = false
    ORDER BY payment_plans.id ASC`)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var price Price
		var popular bool
		if err := rows.Scan(&price.Title, &price.Price, &price.TeamMembers, &price.Projects, &price.Agents, &price.Environments, &price.Requests, &price.SupportType, &popular); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
		}
		if popular {
			price.SubTitle = "Most Popular"
		}
		prices = append(prices, price)
	}

	return prices, nil
}

func (s *System) GetFree() Price {
	return Price{
		Title:        "Free",
		Price:        0,
		TeamMembers:  1,
		Projects:     1,
		Agents:       1,
		Environments: 2,
		Requests:     50000,
		SupportType:  "Community",
	}
}

func (s *System) GetStartup() Price {
	return Price{
		Title:        "Startup",
		SubTitle:     "Most Popular",
		Price:        15,
		TeamMembers:  20,
		Projects:     5,
		Agents:       2,
		Environments: 2,
		Requests:     1000000,
		SupportType:  "Community",
		Extras: []Extra{
			{
				Title:    "A/B traffic testing",
				Launched: false,
			},
		},
	}
}

func (s *System) GetPro() Price {
	return Price{
		Title:        "Pro",
		Price:        50,
		TeamMembers:  50,
		Projects:     10,
		Agents:       2,
		Environments: 3,
		Requests:     5000000,
		SupportType:  "Extended",
		Extras: []Extra{
			{
				Title:    "A/B traffic testing",
				Launched: false,
			},
		},
	}
}

func (s *System) GetEnterprise() Price {
	return Price{
		Title:        "Enterprise",
		Price:        200,
		Agents:       5,
		Environments: 5,
		Requests:     20000000,
		SupportType:  "Priority",
		Extras: []Extra{
			{
				Title:    "A/B traffic testing",
				Launched: false,
			},
		},
	}
}
