package flags

func (s *System) GetAgentFlags(companyId, agentId, environmentId string) (Response, error) {
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

	return res, nil
}
