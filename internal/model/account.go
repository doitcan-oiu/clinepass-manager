package model

type Account struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Password        string `json:"password,omitempty"`
	RecoveryEmail   string `json:"recovery_email"`
	Proxy           string `json:"proxy"`
	FingerprintSeed int    `json:"fingerprint_seed"`
	Status          string `json:"status"`
	WorkspaceID     string `json:"workspace_id"`
	APIKey          string `json:"api_key"`
	UserID          string `json:"user_id"`
	CookiesJSON     string `json:"cookies_json"`
	CookieHeader    string `json:"cookie_header"`
	PaymentURL      string `json:"payment_url"`
	LastError       string `json:"last_error"`
	LastLoginAt     int64  `json:"last_login_at"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	PaidAt          int64  `json:"paid_at"`
	BatchID         string `json:"batch_id"`
	BatchName       string `json:"batch_name,omitempty"`
	LoginProvider   string `json:"login_provider"`
	HasCookies      bool   `json:"has_cookies,omitempty"`
	HasAPIKey       bool   `json:"has_api_key,omitempty"`
}

type AccountPublic struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	RecoveryEmail   string `json:"recovery_email"`
	Proxy           string `json:"proxy"`
	FingerprintSeed int    `json:"fingerprint_seed"`
	Status          string `json:"status"`
	WorkspaceID     string `json:"workspace_id"`
	APIKey          string `json:"api_key"`
	UserID          string `json:"user_id"`
	CookiesJSON     string `json:"cookies_json"`
	CookieHeader    string `json:"cookie_header"`
	PaymentURL      string `json:"payment_url"`
	LastError       string `json:"last_error"`
	LastLoginAt     int64  `json:"last_login_at"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	HasPassword     bool   `json:"has_password"`
	PaidAt          int64  `json:"paid_at"`
	BatchID         string `json:"batch_id"`
	BatchName       string `json:"batch_name,omitempty"`
	LoginProvider   string `json:"login_provider"`
	HasCookies      bool   `json:"has_cookies,omitempty"`
	HasAPIKey       bool   `json:"has_api_key,omitempty"`
}

func (a Account) Public() AccountPublic {
	return AccountPublic{
		ID:              a.ID,
		Email:           a.Email,
		RecoveryEmail:   a.RecoveryEmail,
		Proxy:           a.Proxy,
		FingerprintSeed: a.FingerprintSeed,
		Status:          a.Status,
		WorkspaceID:     a.WorkspaceID,
		APIKey:          a.APIKey,
		UserID:          a.UserID,
		CookiesJSON:     a.CookiesJSON,
		CookieHeader:    a.CookieHeader,
		PaymentURL:      a.PaymentURL,
		LastError:       a.LastError,
		LastLoginAt:     a.LastLoginAt,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
		HasPassword:     a.Password != "",
		PaidAt:          a.PaidAt,
		BatchID:         a.BatchID,
		BatchName:       a.BatchName,
		LoginProvider:   NormalizeLoginProvider(a.LoginProvider),
		HasCookies:      a.HasCookies || a.CookieHeader != "" || a.CookiesJSON != "",
		HasAPIKey:       a.HasAPIKey || a.APIKey != "",
	}
}

func (a Account) ListPublic() AccountPublic {
	p := a.Public()
	p.CookiesJSON = ""
	p.CookieHeader = ""
	p.APIKey = ""
	return p
}

type CreateAccountInput struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	RecoveryEmail string `json:"recovery_email"`
	Proxy         string `json:"proxy"`
	Raw           string `json:"raw"`
	BatchID       string `json:"batch_id"`
	LoginProvider string `json:"login_provider"`
}

type JobEvent struct {
	JobID     string `json:"job_id"`
	AccountID string `json:"account_id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Time      int64  `json:"time"`
}

type Job struct {
	ID        string     `json:"id"`
	AccountID string     `json:"account_id"`
	Email     string     `json:"email"`
	Kind      string     `json:"kind,omitempty"`
	AutoPay   bool       `json:"auto_pay,omitempty"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	Logs      []JobEvent `json:"logs"`
	StartedAt int64      `json:"started_at"`
	EndedAt   int64      `json:"ended_at,omitempty"`
}

type Settings struct {
	Proxy                   string   `json:"proxy"`
	Headless                bool     `json:"headless"`
	InviteURL               string   `json:"invite_url,omitempty"`
	UsageJSURL              string   `json:"usage_js_url,omitempty"`
	HeroSMSAPIKey           string   `json:"hero_sms_api_key,omitempty"`
	HeroSMSService          string   `json:"hero_sms_service,omitempty"`
	HeroSMSCountry          int      `json:"hero_sms_country,omitempty"`
	HeroSMSMaxPrice         float64  `json:"hero_sms_max_price,omitempty"`
	CloakVersion            string   `json:"cloak_version,omitempty"`
	CloakLicenseKey         string   `json:"cloak_license_key,omitempty"`
	AmzKeysHost             string   `json:"amzkeys_host,omitempty"`
	AmzKeysAppID            string   `json:"amzkeys_app_id,omitempty"`
	AmzKeysAppKey           string   `json:"amzkeys_app_key,omitempty"`
	AmzKeysPrivateKey       string   `json:"amzkeys_private_key,omitempty"`
	AmzKeysCardType         int      `json:"amzkeys_card_type,omitempty"`
	AmzKeysCardAmount       float64  `json:"amzkeys_card_amount,omitempty"`
	MaxConcurrent           int      `json:"max_concurrent,omitempty"`
	MaxRetries              int      `json:"max_retries"`
	AccountRPM              int      `json:"account_rpm"`
	APIProxy                bool     `json:"api_proxy"`
	UsageRefreshSec         int      `json:"usage_refresh_sec"`
	UsageRefreshConcurrency int      `json:"usage_refresh_concurrency"`
	EmailSuffixBlacklist    []string `json:"email_suffix_blacklist,omitempty"`
	ProviderMode            string   `json:"provider_mode,omitempty"`
	ProviderValue           string   `json:"provider_value,omitempty"`
}

type AmzKeysCard struct {
	CardNo        string       `json:"card_no"`
	CVV           string       `json:"cvv"`
	ValidDate     string       `json:"valid_date"`
	RequestID     string       `json:"request_id,omitempty"`
	CardType      int          `json:"card_type,omitempty"`
	TaskID        string       `json:"task_id,omitempty"`
	RAM           string       `json:"ram,omitempty"`
	TaskStartedAt int64        `json:"task_started_at,omitempty"`
	Amount        float64      `json:"amount,omitempty"`
	PayCount      int          `json:"pay_count,omitempty"`
	InUse         int          `json:"in_use,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	LastErrorAt   int64        `json:"last_error_at,omitempty"`
	Next          *AmzKeysCard `json:"next,omitempty"`
}

func (c AmzKeysCard) Ready() bool {
	return c.CardNo != "" && c.CVV != ""
}

func (c AmzKeysCard) Pending() bool {
	return !c.Ready() && c.TaskID != "" && c.RAM != ""
}
