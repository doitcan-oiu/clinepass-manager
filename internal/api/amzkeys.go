package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"opencode-go-manager/internal/amzkeys"
	"opencode-go-manager/internal/model"
)

func amzKeysHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return amzkeys.DefaultHost
	}
	return host
}

func amzKeysCardType(n int) int {
	if n <= 0 {
		return amzkeys.DefaultCardType
	}
	return n
}

func amzKeysCardAmount(n float64) float64 {
	if n <= 0 {
		return amzkeys.DefaultAmount
	}
	return n
}

func amzKeysConfigured(st model.Settings) bool {
	return amzkeys.Ready(st.AmzKeysHost, st.AmzKeysAppID, st.AmzKeysAppKey, st.AmzKeysPrivateKey, amzKeysCardType(st.AmzKeysCardType), amzKeysCardAmount(st.AmzKeysCardAmount)) == nil
}

func (s *Server) amzKeysClient() (*amzkeys.Client, error) {
	st, err := s.store.GetSettings()
	if err != nil {
		st = model.Settings{}
	}
	if err := amzkeys.Ready(st.AmzKeysHost, st.AmzKeysAppID, st.AmzKeysAppKey, st.AmzKeysPrivateKey, amzKeysCardType(st.AmzKeysCardType), amzKeysCardAmount(st.AmzKeysCardAmount)); err != nil {
		return nil, err
	}
	return amzkeys.New(amzKeysHost(st.AmzKeysHost), st.AmzKeysAppID, st.AmzKeysAppKey, st.AmzKeysPrivateKey, amzKeysCardType(st.AmzKeysCardType), amzKeysCardAmount(st.AmzKeysCardAmount)), nil
}

func (s *Server) amzKeysStatus(w http.ResponseWriter, r *http.Request) {
	c, err := s.amzKeysClient()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请先保存 amzkeys卡台："+err.Error())
		return
	}
	st, err := c.Status()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func amzKeysCardLast4(s *Server) string {
	c, err := s.store.GetAmzKeysCard()
	if err != nil || !c.Ready() {
		return ""
	}
	return amzkeys.Last4(c.CardNo)
}

func amzKeysCardJSON(c model.AmzKeysCard, reused bool) map[string]any {
	return map[string]any{
		"card_no":    c.CardNo,
		"cvv":        c.CVV,
		"valid_date": c.ValidDate,
		"expiry":     amzkeys.ExpiryMMYY(c.ValidDate),
		"last4":      amzkeys.Last4(c.CardNo),
		"request_id": c.RequestID,
		"card_type":  c.CardType,
		"reused":     reused,
	}
}

func (s *Server) checkoutAmzKeysCard(replace bool) (model.AmzKeysCard, bool, error) {
	s.amzCardMu.Lock()
	defer s.amzCardMu.Unlock()
	if !replace {
		if cur, err := s.store.GetAmzKeysCard(); err == nil && cur.Ready() {
			return cur, true, nil
		}
	} else if err := s.store.SetAmzKeysCard(model.AmzKeysCard{}); err != nil {
		return model.AmzKeysCard{}, false, err
	}
	c, err := s.amzKeysClient()
	if err != nil {
		return model.AmzKeysCard{}, false, err
	}
	created, err := c.CreateCard()
	if err != nil {
		return model.AmzKeysCard{}, false, err
	}
	card := model.AmzKeysCard{
		CardNo:    strings.TrimSpace(created.CardNo),
		CVV:       strings.TrimSpace(created.CVV),
		ValidDate: strings.TrimSpace(created.ValidDate),
		RequestID: strings.TrimSpace(created.RequestID),
		CardType:  created.CardType,
	}
	if !card.Ready() {
		return model.AmzKeysCard{}, false, fmt.Errorf("开卡没有返回卡号或 CVV")
	}
	if err := s.store.SetAmzKeysCard(card); err != nil {
		return model.AmzKeysCard{}, false, err
	}
	return card, false, nil
}

func (s *Server) amzKeysCheckoutCard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Replace bool `json:"replace"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	card, reused, err := s.checkoutAmzKeysCard(in.Replace)
	if err != nil {
		if strings.Contains(err.Error(), "缺少") || strings.Contains(err.Error(), "私钥") || strings.Contains(err.Error(), "卡段") || strings.Contains(err.Error(), "金额") {
			writeErr(w, http.StatusBadRequest, "自动支付需要先在设置里配好 amzkeys卡台："+err.Error())
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, amzKeysCardJSON(card, reused))
}

func (s *Server) amzKeysClearCard(w http.ResponseWriter, r *http.Request) {
	s.amzCardMu.Lock()
	defer s.amzCardMu.Unlock()
	if err := s.store.SetAmzKeysCard(model.AmzKeysCard{}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) amzKeysAuthCodes(w http.ResponseWriter, r *http.Request) {
	c, err := s.amzKeysClient()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := c.AuthCodes()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	last4 := strings.TrimSpace(r.URL.Query().Get("last4"))
	if last4 != "" {
		filtered := items[:0]
		for _, it := range items {
			if strings.TrimSpace(it.CardNoLast4) == last4 {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": items})
}

func (s *Server) requireAutoPay(autoPay bool) error {
	if !autoPay {
		return nil
	}
	st, err := s.store.GetSettings()
	if err != nil {
		st = model.Settings{}
	}
	if err := amzkeys.Ready(st.AmzKeysHost, st.AmzKeysAppID, st.AmzKeysAppKey, st.AmzKeysPrivateKey, amzKeysCardType(st.AmzKeysCardType), amzKeysCardAmount(st.AmzKeysCardAmount)); err != nil {
		return fmt.Errorf("自动支付需要先在设置里配好 amzkeys卡台：%w", err)
	}
	return nil
}

func readAutoPay(r *http.Request) bool {
	if r.Body == nil {
		return false
	}
	var in struct {
		AutoPay bool `json:"auto_pay"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	return in.AutoPay
}
