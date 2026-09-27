package model

// Dashboard combines gateway request history with separately sourced account
// billing. Dollar values are nullable: missing billing is not a zero charge.
type Dashboard struct {
	Range            string                `json:"range"`
	GeneratedAt      int64                 `json:"generated_at"`
	Since            int64                 `json:"since"`
	Until            int64                 `json:"until"`
	Timezone         string                `json:"timezone"`
	HistorySince     int64                 `json:"history_since"`
	HistoryPartial   bool                  `json:"history_partial"`
	BillingAvailable bool                  `json:"billing_available"`
	BillingSource    string                `json:"billing_source"`
	BillingNote      string                `json:"billing_note"`
	BillingSyncedAt  *int64                `json:"billing_synced_at"`
	Summary          DashboardSummary      `json:"summary"`
	Timeline         []DashboardPoint      `json:"timeline"`
	Channels         []DashboardChannel    `json:"channels"`
	Models           []DashboardModel      `json:"models"`
	Activity         []DashboardActivity   `json:"activity"`
	Availability     DashboardAvailability `json:"availability"`
}

type DashboardSummary struct {
	Accounts          int      `json:"accounts"`
	Requests          int64    `json:"requests"`
	Success           int64    `json:"success"`
	Error             int64    `json:"error"`
	Processing        int64    `json:"processing"`
	SuccessRate       float64  `json:"success_rate"`
	TotalTokens       int64    `json:"total_tokens"`
	InputTokens       int64    `json:"input_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	CacheRead         int64    `json:"cache_read"`
	CacheWrite        int64    `json:"cache_write"`
	CacheHitRate      float64  `json:"cache_hit_rate"`
	CostUSD           *float64 `json:"cost_usd"`
	AvgRequestCostUSD *float64 `json:"avg_request_cost_usd"`
	AvgTTFTMS         *float64 `json:"avg_ttft_ms"`
	TTFTSamples       int64    `json:"ttft_samples"`
	AvgOutputTPS      *float64 `json:"avg_output_tps"`
	OutputSamples     int64    `json:"output_samples"`
}

type DashboardPoint struct {
	Timestamp   int64    `json:"timestamp"`
	Requests    int64    `json:"requests"`
	TotalTokens int64    `json:"total_tokens"`
	CostUSD     *float64 `json:"cost_usd"`
}

type DashboardChannel struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Requests    int64   `json:"requests"`
	Success     int64   `json:"success"`
	SuccessRate float64 `json:"success_rate"`
	TotalTokens int64   `json:"total_tokens"`
	Share       float64 `json:"share"`
}

type DashboardModel struct {
	ID              string   `json:"id"`
	Requests        int64    `json:"requests"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	ReasoningTokens int64    `json:"reasoning_tokens"`
	CacheRead       int64    `json:"cache_read"`
	CacheWrite      int64    `json:"cache_write"`
	TotalTokens     int64    `json:"total_tokens"`
	CostUSD         *float64 `json:"cost_usd"`
}

type DashboardActivity struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
}

type DashboardAvailability struct {
	AccountsTotal       int `json:"accounts_total"`
	AccountsAvailable   int `json:"accounts_available"`
	AccountsUnavailable int `json:"accounts_unavailable"`
	ModelsTotal         int `json:"models_total"`
	ModelsEnabled       int `json:"models_enabled"`
	KeysTotal           int `json:"keys_total"`
	KeysAvailable       int `json:"keys_available"`
}
