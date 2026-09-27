package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
)

// Dashboard ranges are calendar buckets in UTC, including the current bucket.
// Explicit bounds let clients display the same dates independent of timezone.
func DashboardBounds(period string, now time.Time) (time.Time, time.Duration, int, error) {
	now = now.UTC()
	if period == "24h" {
		return now.Truncate(time.Hour).Add(-23 * time.Hour), time.Hour, 24, nil
	}
	days := map[string]int{"7d": 7, "30d": 30, "90d": 90}[period]
	if days == 0 {
		return time.Time{}, 0, 0, fmt.Errorf("range 只支持 24h、7d、30d、90d")
	}
	return now.Truncate(24*time.Hour).AddDate(0, 0, 1-days), 24 * time.Hour, days, nil
}

func (s *Store) Dashboard(period string, now time.Time) (model.Dashboard, error) {
	if period == "" {
		period = "30d"
	}
	since, step, count, err := DashboardBounds(period, now)
	if err != nil {
		return model.Dashboard{}, err
	}
	out := model.Dashboard{
		Range: period, GeneratedAt: now.UnixMilli(), Since: since.UnixMilli(), Until: now.UnixMilli(), Timezone: "UTC",
		BillingSource: "account_daily_usage",
		BillingNote:   "计费按上游账单日期汇总账号已同步金额，可能包含网关外使用；上游未提供账单时区，日期边界可能与 UTC 请求统计不同。未同步日期不视为零费用。",
		Timeline:      make([]model.DashboardPoint, count), Channels: []model.DashboardChannel{}, Models: []model.DashboardModel{}, Activity: make([]model.DashboardActivity, 180),
	}
	if period == "24h" {
		out.BillingNote = "上游仅提供每日账单，无法准确拆分到最近 24 小时；请切换到 7d、30d 或 90d 查看已同步计费。"
	}
	if err := s.db.QueryRow(`SELECT history_since FROM dashboard_meta WHERE id = 1`).Scan(&out.HistorySince); err != nil {
		return out, err
	}
	out.HistoryPartial = out.HistorySince > out.Since
	for i := range out.Timeline {
		out.Timeline[i].Timestamp = since.Add(time.Duration(i) * step).UnixMilli()
	}
	activityStart := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -179)
	for i := range out.Activity {
		out.Activity[i].Date = activityStart.AddDate(0, 0, i).Format("2006-01-02")
	}
	models := map[string]*model.DashboardModel{}
	for _, m := range gomodel.All() {
		models[m.ID] = &model.DashboardModel{ID: m.ID}
	}
	channels := map[string]*model.DashboardChannel{}
	names, _ := dashboardMetricSQL("")
	rows, err := s.db.Query(`SELECT hour, model, channel, `+strings.Join(names, ",")+` FROM dashboard_hours WHERE hour >= ? AND hour <= ? AND requests > 0`, activityStart.UnixMilli(), now.UTC().Truncate(time.Hour).UnixMilli())
	if err != nil {
		return out, err
	}
	var ttftSum, outputSum float64
	for rows.Next() {
		var hour int64
		var modelID, channel string
		var requests, success, errors, processing, input, output, reasoning, cacheRead, cacheWrite, total, ttft, ttftN, outputN int64
		var outputTPS float64
		if err := rows.Scan(&hour, &modelID, &channel, &requests, &success, &errors, &processing, &input, &output, &reasoning, &cacheRead, &cacheWrite, &total, &ttft, &ttftN, &outputTPS, &outputN); err != nil {
			rows.Close()
			return out, err
		}
		activityIndex := int((hour - activityStart.UnixMilli()) / int64((24 * time.Hour).Milliseconds()))
		if activityIndex >= 0 && activityIndex < len(out.Activity) {
			out.Activity[activityIndex].Requests += requests
		}
		if hour < since.UnixMilli() {
			continue
		}
		pointIndex := int((hour - since.UnixMilli()) / step.Milliseconds())
		if pointIndex < 0 || pointIndex >= count {
			continue
		}
		point := &out.Timeline[pointIndex]
		point.Requests += requests
		point.TotalTokens += total
		st := &out.Summary
		st.Requests += requests
		st.Success += success
		st.Error += errors
		st.Processing += processing
		st.InputTokens += input
		st.OutputTokens += output
		st.CacheRead += cacheRead
		st.CacheWrite += cacheWrite
		st.TotalTokens += total
		st.TTFTSamples += ttftN
		st.OutputSamples += outputN
		ttftSum += float64(ttft)
		outputSum += outputTPS
		modelID = gomodel.Canonical(modelID)
		m := models[modelID]
		if m == nil {
			m = &model.DashboardModel{ID: modelID}
			models[modelID] = m
		}
		m.Requests += requests
		m.InputTokens += input
		m.OutputTokens += output
		m.ReasoningTokens += reasoning
		m.CacheRead += cacheRead
		m.CacheWrite += cacheWrite
		m.TotalTokens += total
		c := channels[channel]
		if c == nil {
			c = &model.DashboardChannel{ID: channel, Name: dashboardChannelName(channel)}
			channels[channel] = c
		}
		c.Requests += requests
		c.Success += success
		c.TotalTokens += total
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.Summary.SuccessRate = dashboardPercent(out.Summary.Success, out.Summary.Requests)
	out.Summary.CacheHitRate = dashboardPercent(out.Summary.CacheRead, out.Summary.InputTokens)
	if out.Summary.TTFTSamples > 0 {
		avg := ttftSum / float64(out.Summary.TTFTSamples)
		out.Summary.AvgTTFTMS = &avg
	}
	if out.Summary.OutputSamples > 0 {
		avg := outputSum / float64(out.Summary.OutputSamples)
		out.Summary.AvgOutputTPS = &avg
	}
	for _, c := range channels {
		c.SuccessRate = dashboardPercent(c.Success, c.Requests)
		c.Share = dashboardPercent(c.Requests, out.Summary.Requests)
		out.Channels = append(out.Channels, *c)
	}
	sort.Slice(out.Channels, func(i, j int) bool {
		if out.Channels[i].Requests == out.Channels[j].Requests {
			return out.Channels[i].ID < out.Channels[j].ID
		}
		return out.Channels[i].Requests > out.Channels[j].Requests
	})
	if period != "24h" {
		if err := s.dashboardBilling(&out, models, since, now); err != nil {
			return out, err
		}
	}
	for _, m := range models {
		out.Models = append(out.Models, *m)
	}
	sort.Slice(out.Models, func(i, j int) bool {
		a, b := out.Models[i], out.Models[j]
		if a.CostUSD != nil || b.CostUSD != nil {
			if a.CostUSD == nil {
				return false
			}
			if b.CostUSD == nil {
				return true
			}
			if *a.CostUSD != *b.CostUSD {
				return *a.CostUSD > *b.CostUSD
			}
		}
		if a.TotalTokens != b.TotalTokens {
			return a.TotalTokens > b.TotalTokens
		}
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return a.ID < b.ID
	})
	if len(out.Models) > 10 {
		out.Models = out.Models[:10]
	}
	return out, nil
}

func (s *Store) dashboardBilling(out *model.Dashboard, models map[string]*model.DashboardModel, since, now time.Time) error {
	rows, err := s.db.Query(`SELECT date, model, SUM(usd), MAX(synced_at) FROM dashboard_billing
		WHERE date >= ? AND date <= ? GROUP BY date, model`, since.Format("2006-01-02"), now.UTC().Format("2006-01-02"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var date, id string
		var usd float64
		var synced int64
		if err := rows.Scan(&date, &id, &usd, &synced); err != nil {
			return err
		}
		out.BillingAvailable = true
		if out.BillingSyncedAt == nil || synced*1000 > *out.BillingSyncedAt {
			x := synced * 1000
			out.BillingSyncedAt = &x
		}
		addCost(&out.Summary.CostUSD, usd)
		day, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		index := int(day.Sub(since) / (24 * time.Hour))
		if index >= 0 && index < len(out.Timeline) {
			addCost(&out.Timeline[index].CostUSD, usd)
		}
		m := models[id]
		if m == nil {
			m = &model.DashboardModel{ID: id}
			models[id] = m
		}
		addCost(&m.CostUSD, usd)
	}
	return rows.Err()
}

func addCost(dst **float64, amount float64) {
	if *dst == nil {
		*dst = new(float64)
	}
	**dst += amount
}

func dashboardPercent(n, total int64) float64 {
	if total <= 0 {
		return 0
	}
	p := 100 * float64(n) / float64(total)
	if p > 100 {
		return 100
	}
	return p
}

func dashboardChannelName(id string) string {
	switch id {
	case "openai/chat_completions":
		return "Chat Completions"
	case "openai/responses":
		return "Responses"
	case "anthropic/messages":
		return "Messages"
	case "":
		return "未知协议"
	default:
		return id
	}
}
