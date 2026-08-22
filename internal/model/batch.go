package model

type Batch struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	ExportedAt    int64  `json:"exported_at"`
	ExportedCount int    `json:"exported_count"`
	PaidAt        int64  `json:"paid_at"`
}

type BatchSummary struct {
	Batch
	Total             int `json:"total"`
	Pending           int `json:"pending"`
	Ready             int `json:"ready"`
	Failed            int `json:"failed"`
	PayCount          int `json:"pay_count"`
	CookieCount       int `json:"cookie_count"`
	PaidCount         int `json:"paid_count"`
	UnpaidPayCount    int `json:"unpaid_pay_count"`
	UnpaidCookieCount int `json:"unpaid_cookie_count"`
}

type PayLink struct {
	Email string `json:"email"`
	URL   string `json:"payment_url"`
}

type CreateBatchInput struct {
	Name          string `json:"name"`
	Text          string `json:"text"`
	LoginProvider string `json:"login_provider"`
}
