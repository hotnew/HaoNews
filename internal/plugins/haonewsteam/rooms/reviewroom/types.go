package reviewroom

type ReviewMessage struct {
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary,omitempty"`
	Checklist      []string `json:"checklist,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
}

type DecisionMessage struct {
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary,omitempty"`
	Decision  string   `json:"decision,omitempty"`
	NextSteps []string `json:"next_steps,omitempty"`
}

type RiskMessage struct {
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary,omitempty"`
	Impact     string   `json:"impact,omitempty"`
	Mitigation []string `json:"mitigation,omitempty"`
}
