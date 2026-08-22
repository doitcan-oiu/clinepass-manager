package model

type RequestLog struct {
	ID              int64  `json:"id"`
	CreatedAt       int64  `json:"created_at"`
	Model           string `json:"model"`
	APIFormat       string `json:"api_format"`
	Stream          bool   `json:"stream"`
	AccountID       string `json:"account_id"`
	AccountEmail    string `json:"account_email"`
	Status          string `json:"status"`
	HTTPStatus      int    `json:"http_status"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	ReasoningTokens int    `json:"reasoning_tokens"`
	CacheRead       int    `json:"cache_read"`
	CacheWrite      int    `json:"cache_write"`
	TotalTokens     int    `json:"total_tokens"`
	DurationMS      int    `json:"duration_ms"`
	TTFTMS          int    `json:"ttft_ms"`
	Retries         int    `json:"retries"`
	Error           string `json:"error"`
}

type RequestLogPage struct {
	Items    []RequestLog    `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Stats    RequestLogStats `json:"stats"`
}

type RequestLogFilter struct {
	Model  string
	Email  string
	Status string
	Stream string
}

type RequestLogStats struct {
	RPM1m       int      `json:"rpm_1m"`
	TPM1m       int      `json:"tpm_1m"`
	RPM5m       float64  `json:"rpm_5m"`
	TPM5m       float64  `json:"tpm_5m"`
	Requests1h  int      `json:"requests_1h"`
	Tokens1h    int      `json:"tokens_1h"`
	Requests24h int      `json:"requests_24h"`
	Tokens24h   int      `json:"tokens_24h"`
	Success1h   int      `json:"success_1h"`
	Error1h     int      `json:"error_1h"`
	Processing  int      `json:"processing"`
	Models      []string `json:"models"`
}
