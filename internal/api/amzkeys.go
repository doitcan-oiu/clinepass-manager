package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
	if err != nil {
		return ""
	}
	if c.Ready() {
		return amzkeys.Last4(c.CardNo)
	}
	if c.Next != nil && c.Next.Ready() {
		return amzkeys.Last4(c.Next.CardNo)
	}
	return ""
}

func amzKeysCardView(s *Server) (last4 string, pending bool, payCount, maxPays int, nextLast4 string, nextPending bool) {
	c, err := s.store.GetAmzKeysCard()
	if err != nil {
		return
	}
	if c.Ready() {
		last4 = amzkeys.Last4(c.CardNo)
		payCount = c.PayCount
		maxPays = amzkeys.MaxPays(c.Amount)
	} else if c.Pending() {
		pending = true
	}
	if c.Next != nil {
		if c.Next.Ready() {
			nextLast4 = amzkeys.Last4(c.Next.CardNo)
		} else if c.Next.Pending() {
			nextPending = true
		}
	}
	return
}

func cardRemaining(c model.AmzKeysCard) int {
	return amzkeys.RemainingPays(c.Amount, c.PayCount)
}

func cardExhausted(c model.AmzKeysCard) bool {
	return c.Ready() && cardRemaining(c) <= 0
}

func cardUsable(c model.AmzKeysCard) bool {
	return c.Ready() && !cardExhausted(c)
}

func cardPendingFresh(c model.AmzKeysCard) bool {
	return c.Pending() && !amzkeys.TaskStale(c.TaskStartedAt)
}

func cardMatchLast4(c model.AmzKeysCard, last4 string) bool {
	return last4 != "" && amzkeys.Last4(c.CardNo) == last4
}

func publicCard(c model.AmzKeysCard) model.AmzKeysCard {
	c.Next = nil
	return c
}

func amzHasStock(c model.AmzKeysCard) bool {
	if cardUsable(c) || cardPendingFresh(c) {
		return true
	}
	if c.Next != nil && (cardUsable(*c.Next) || cardPendingFresh(*c.Next)) {
		return true
	}
	return false
}

func amzShouldWarmNext(c model.AmzKeysCard) bool {
	if !cardUsable(c) {
		return !amzHasStock(c)
	}
	if cardRemaining(c) > 1 {
		return false
	}
	if c.Next != nil && (cardUsable(*c.Next) || cardPendingFresh(*c.Next)) {
		return false
	}
	return true
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
		"pay_count":  c.PayCount,
		"max_pays":   amzkeys.MaxPays(c.Amount),
		"remaining":  cardRemaining(c),
	}
}

func (s *Server) tryReserveReadyLocked() (model.AmzKeysCard, bool, error) {
	cur, err := s.store.GetAmzKeysCard()
	if err != nil {
		return model.AmzKeysCard{}, false, err
	}
	if cardUsable(cur) {
		cur.PayCount++
		cur.InUse++
		if err := s.store.SetAmzKeysCard(cur); err != nil {
			return model.AmzKeysCard{}, false, err
		}
		return publicCard(cur), true, nil
	}
	if cur.Next != nil && cardUsable(*cur.Next) {
		cur.Next.PayCount++
		cur.Next.InUse++
		if err := s.store.SetAmzKeysCard(cur); err != nil {
			return model.AmzKeysCard{}, false, err
		}
		return publicCard(*cur.Next), true, nil
	}
	return model.AmzKeysCard{}, false, nil
}

func (s *Server) discardCurrentLocked() error {
	cur, err := s.store.GetAmzKeysCard()
	if err != nil {
		return err
	}
	if cur.Next != nil && (cur.Next.Ready() || cur.Next.Pending()) {
		next := *cur.Next
		next.Next = nil
		return s.store.SetAmzKeysCard(next)
	}
	return s.store.SetAmzKeysCard(model.AmzKeysCard{})
}

func (s *Server) recycleCardLocked(cur *model.AmzKeysCard) {
	if cur.Next != nil && cardExhausted(*cur.Next) && cur.Next.InUse == 0 {
		cur.Next = nil
	}
	if cur.Ready() && cardExhausted(*cur) && cur.InUse == 0 {
		if cur.Next != nil {
			next := *cur.Next
			next.Next = nil
			*cur = next
			return
		}
		*cur = model.AmzKeysCard{}
	}
}

func (s *Server) checkoutAmzKeysCard(replace bool) (model.AmzKeysCard, bool, error) {
	s.amzCardMu.Lock()
	if replace {
		if err := s.discardCurrentLocked(); err != nil {
			s.amzCardMu.Unlock()
			return model.AmzKeysCard{}, false, err
		}
	}
	if card, ok, err := s.tryReserveReadyLocked(); ok || err != nil {
		warm := false
		if err == nil {
			if cur, gerr := s.store.GetAmzKeysCard(); gerr == nil {
				warm = amzShouldWarmNext(cur)
			}
		}
		s.amzCardMu.Unlock()
		if warm {
			s.WarmAmzKeysCard()
		}
		return card, true, err
	}
	asNext, taskID, ram, err := s.beginAmzKeysCreateLocked()
	s.amzCardMu.Unlock()
	if err != nil {
		return model.AmzKeysCard{}, false, err
	}
	if taskID != "" {
		if _, err := s.finishAmzKeysCreate(taskID, ram, asNext); err != nil {
			return model.AmzKeysCard{}, false, err
		}
	}
	s.amzCardMu.Lock()
	card, ok, err := s.tryReserveReadyLocked()
	warm := false
	if err == nil {
		if cur, gerr := s.store.GetAmzKeysCard(); gerr == nil {
			warm = amzShouldWarmNext(cur)
		}
	}
	s.amzCardMu.Unlock()
	if err != nil {
		return model.AmzKeysCard{}, false, err
	}
	if !ok {
		return model.AmzKeysCard{}, false, fmt.Errorf("没有可用的虚拟卡")
	}
	if warm {
		s.WarmAmzKeysCard()
	}
	return card, false, nil
}

func (s *Server) beginAmzKeysCreateLocked() (asNext bool, taskID, ram string, err error) {
	cur, _ := s.store.GetAmzKeysCard()
	if cardUsable(cur) {
		return false, "", "", nil
	}
	if cardPendingFresh(cur) {
		return false, cur.TaskID, cur.RAM, nil
	}
	if cur.Next != nil && cardUsable(*cur.Next) {
		return true, "", "", nil
	}
	if cur.Next != nil && cardPendingFresh(*cur.Next) {
		return true, cur.Next.TaskID, cur.Next.RAM, nil
	}
	if cur.Ready() && cardExhausted(cur) && cur.InUse == 0 {
		next := cur.Next
		cur = model.AmzKeysCard{Next: next}
		if err := s.store.SetAmzKeysCard(cur); err != nil {
			return false, "", "", err
		}
	}
	asNext = cur.Ready()
	c, err := s.amzKeysClient()
	if err != nil {
		return false, "", "", err
	}
	taskID, ram, err = c.SubmitCreate()
	if err != nil {
		return false, "", "", err
	}
	pending := model.AmzKeysCard{
		TaskID:        taskID,
		RAM:           ram,
		TaskStartedAt: time.Now().Unix(),
		Amount:        c.Amount,
	}
	if asNext {
		n := pending
		cur.Next = &n
		if err := s.store.SetAmzKeysCard(cur); err != nil {
			return false, "", "", err
		}
		return true, taskID, ram, nil
	}
	pending.Next = cur.Next
	if err := s.store.SetAmzKeysCard(pending); err != nil {
		return false, "", "", err
	}
	return false, taskID, ram, nil
}

func (s *Server) finishAmzKeysCreate(taskID, ram string, asNext bool) (model.AmzKeysCard, error) {
	if taskID == "" || ram == "" {
		if cur, err := s.store.GetAmzKeysCard(); err == nil {
			if asNext && cur.Next != nil && cur.Next.Ready() {
				return publicCard(*cur.Next), nil
			}
			if cur.Ready() {
				return publicCard(cur), nil
			}
		}
		return model.AmzKeysCard{}, fmt.Errorf("没有开卡任务")
	}
	c, err := s.amzKeysClient()
	if err != nil {
		return model.AmzKeysCard{}, err
	}
	created, err := c.WaitTask(taskID, ram)
	if err != nil {
		s.amzCardMu.Lock()
		if cur, gerr := s.store.GetAmzKeysCard(); gerr == nil {
			if asNext && cur.Next != nil && cur.Next.TaskID == taskID && !cur.Next.Ready() {
				cur.Next = nil
				_ = s.store.SetAmzKeysCard(cur)
			} else if !asNext && cur.TaskID == taskID && !cur.Ready() {
				_ = s.store.SetAmzKeysCard(model.AmzKeysCard{Next: cur.Next})
			}
		}
		s.amzCardMu.Unlock()
		return model.AmzKeysCard{}, err
	}
	amount := created.OpenCardAmount
	if amount <= 0 {
		amount = c.Amount
	}
	card := model.AmzKeysCard{
		CardNo:        strings.TrimSpace(created.CardNo),
		CVV:           strings.TrimSpace(created.CVV),
		ValidDate:     strings.TrimSpace(created.ValidDate),
		RequestID:     strings.TrimSpace(created.RequestID),
		CardType:      created.CardType,
		TaskID:        taskID,
		RAM:           ram,
		TaskStartedAt: time.Now().Unix(),
		Amount:        amount,
	}
	if !card.Ready() {
		return model.AmzKeysCard{}, fmt.Errorf("开卡没有返回卡号或 CVV")
	}
	s.amzCardMu.Lock()
	defer s.amzCardMu.Unlock()
	cur, gerr := s.store.GetAmzKeysCard()
	if gerr != nil {
		return model.AmzKeysCard{}, gerr
	}
	if asNext {
		if cur.Next != nil && cur.Next.Ready() && cur.Next.CardNo != card.CardNo {
			return publicCard(*cur.Next), nil
		}
		n := card
		cur.Next = &n
		if err := s.store.SetAmzKeysCard(cur); err != nil {
			return model.AmzKeysCard{}, err
		}
		return publicCard(card), nil
	}
	if cur.Ready() && cur.CardNo != card.CardNo {
		return publicCard(cur), nil
	}
	card.PayCount = cur.PayCount
	card.InUse = cur.InUse
	card.Next = cur.Next
	if err := s.store.SetAmzKeysCard(card); err != nil {
		return model.AmzKeysCard{}, err
	}
	return publicCard(card), nil
}

func (s *Server) WarmAmzKeysCard() {
	go s.warmAmzKeysCard()
}

func (s *Server) warmAmzKeysCard() {
	if _, err := s.amzKeysClient(); err != nil {
		return
	}
	s.amzCardMu.Lock()
	cur, err := s.store.GetAmzKeysCard()
	if err == nil && amzHasStock(cur) && !amzShouldWarmNext(cur) {
		s.amzCardMu.Unlock()
		return
	}
	asNext, taskID, ram, err := s.beginAmzKeysCreateLocked()
	s.amzCardMu.Unlock()
	if err != nil {
		log.Printf("提前开卡失败: %v", err)
		return
	}
	if taskID == "" {
		return
	}
	if _, err := s.finishAmzKeysCreate(taskID, ram, asNext); err != nil {
		log.Printf("提前开卡等待失败: %v", err)
	}
}

func (s *Server) releaseAmzKeysCard(last4 string, success, rejected bool) {
	last4 = strings.TrimSpace(last4)
	s.amzCardMu.Lock()
	cur, err := s.store.GetAmzKeysCard()
	if err != nil {
		s.amzCardMu.Unlock()
		return
	}
	apply := func(c *model.AmzKeysCard) bool {
		if !cardMatchLast4(*c, last4) && last4 != "" {
			return false
		}
		if last4 == "" && !c.Ready() {
			return false
		}
		if c.InUse > 0 {
			c.InUse--
		}
		if !success && !rejected && c.PayCount > 0 {
			c.PayCount--
		}
		return true
	}
	hit := apply(&cur)
	if !hit && cur.Next != nil {
		hit = apply(cur.Next)
	}
	if !hit && last4 == "" && cur.Ready() {
		hit = apply(&cur)
	}
	if hit {
		s.recycleCardLocked(&cur)
		_ = s.store.SetAmzKeysCard(cur)
	}
	warm := !amzHasStock(cur) || amzShouldWarmNext(cur)
	s.amzCardMu.Unlock()
	if warm {
		s.WarmAmzKeysCard()
	}
}

func (s *Server) amzKeysWarmCard(w http.ResponseWriter, r *http.Request) {
	if _, err := s.amzKeysClient(); err != nil {
		writeErr(w, http.StatusBadRequest, "自动支付需要先在设置里配好 amzkeys卡台："+err.Error())
		return
	}
	s.WarmAmzKeysCard()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"pending": true,
		"last4":   amzKeysCardLast4(s),
	})
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

func (s *Server) amzKeysReleaseCard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Last4    string `json:"last4"`
		Success  bool   `json:"success"`
		Rejected bool   `json:"rejected"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.releaseAmzKeysCard(in.Last4, in.Success, in.Rejected)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) amzKeysClearCard(w http.ResponseWriter, r *http.Request) {
	s.amzCardMu.Lock()
	err := s.store.SetAmzKeysCard(model.AmzKeysCard{})
	s.amzCardMu.Unlock()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.WarmAmzKeysCard()
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
