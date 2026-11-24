package web

type PageState struct {
	AuthEnabled    bool
	Authenticated  bool
	Username       string
	ScopeLabel     string
	HasReadScope   bool
	CSRFToken      string
	ExpiresAt      string
	PMSSConfigured bool
	PMSSAPIBaseURL string
	Error          string
}
