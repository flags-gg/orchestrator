package pricing

import (
	"database/sql"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Extra struct {
	Title    string `json:"title,omitempty"`
	Launched bool   `json:"launched,omitempty"`
}
type Stripe struct {
	PriceID        string `json:"price_id,omitempty"`
	PriceString    *sql.NullString
	DevPriceID     string `json:"dev_price_id,omitempty"`
	DevPriceString *sql.NullString
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
	Stripe       Stripe  `json:"stripe,omitempty"`
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
      payment_plans.stripe_id,
      payment_plans.stripe_id_dev,
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
		var stripe Stripe
		if err := rows.Scan(&price.Title, &price.Price, &price.TeamMembers, &price.Projects, &price.Agents, &price.Environments, &price.Requests, &price.SupportType, &stripe.PriceString, &stripe.DevPriceString, &popular); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
		}
		if popular {
			price.SubTitle = "Most Popular"
		}

		if stripe.PriceString != nil && stripe.PriceString.Valid {
			price.Stripe.PriceID = stripe.PriceString.String
		}
		if stripe.DevPriceString != nil && stripe.DevPriceString.Valid {
			price.Stripe.DevPriceID = stripe.DevPriceString.String
		}

		price.Stripe = stripe
		prices = append(prices, price)
	}

	return prices, nil
}

func (s *System) GetPrice(title string) (Price, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return Price{}, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var price Price
	var stripe Stripe
	row := client.QueryRow(s.Context, `
    SELECT
      payment_plans.name,
      payment_plans.price,
      payment_plans.team_members,
      payment_plans.projects,
      payment_plans.agents,
      payment_plans.environments,
      payment_plans.requests,
      payment_plans.support_category,
      payment_plans.stripe_id,
      payment_plans.stripe_id_dev
    FROM public.payment_plans
    WHERE payment_plans.name = $1`, title)
	if err := row.Scan(&price.Title, &price.Price, &price.TeamMembers, &price.Projects, &price.Agents, &price.Environments, &price.Requests, &price.SupportType, &stripe.PriceID, &stripe.DevPriceID); err != nil {
		return Price{}, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
	}

	price.Stripe = stripe
	return price, nil
}

func (s *System) GetFree() Price {
	price, err := s.GetPrice("free")
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get free price: %v", err)
		return Price{}
	}
	price.Title = cases.Title(language.English).String(price.Title)
	return price
}

func (s *System) GetStartup() Price {
	price, err := s.GetPrice("startup")
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get startup price: %v", err)
		return Price{}
	}
	price.Title = cases.Title(language.English).String(price.Title)
	price.SubTitle = "Most Popular"
	price.Extras = append(price.Extras, Extra{
		Title:    "A/B traffic testing",
		Launched: false,
	})
	return price
}

func (s *System) GetPro() Price {
	price, err := s.GetPrice("pro")
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get startup price: %v", err)
		return Price{}
	}
	price.Title = cases.Title(language.English).String(price.Title)
	price.Extras = append(price.Extras, Extra{
		Title:    "A/B traffic testing",
		Launched: false,
	})
	return price
}

func (s *System) GetEnterprise() Price {
	price, err := s.GetPrice("enterprise")
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get startup price: %v", err)
		return Price{}
	}
	price.Title = cases.Title(language.English).String(price.Title)
	price.Extras = append(price.Extras, Extra{
		Title:    "A/B traffic testing",
		Launched: false,
	})
	return price
}
