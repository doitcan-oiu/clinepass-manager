package herosms

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePricesAndOffers(t *testing.T) {
	names := map[int]string{6: "印度尼西亚", 16: "英国"}
	got, err := parsePrices([]byte(`{
		"6": {"ot": {"cost": 0.15, "count": 80}},
		"16": {"ot": [{"cost": 0.4, "count": 3}, {"cost": 0.9, "count": 10}]}
	}`), "ot", names, map[int]int{6: 62})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("countries=%d", len(got))
	}
	if got[0].ID != 6 || got[0].Name != "印度尼西亚" || got[0].Quotes[0].Price != 0.15 || got[0].PhoneCode != 62 {
		t.Fatalf("%+v", got[0])
	}
	if len(got[1].Quotes) != 2 || got[1].Quotes[0].Price != 0.4 {
		t.Fatalf("quotes %+v", got[1].Quotes)
	}

	offers, err := parseOffers([]byte(`{"data":[
		{"country":6,"countryName":"Indonesia","price":0.15,"count":12,"service":"ot","countryPhoneCode":62},
		{"country":6,"price":0.22,"count":4,"service":"ot"}
	]}`), "ot")
	if err != nil || len(offers) != 1 || len(offers[0].Quotes) != 2 {
		t.Fatalf("%+v %v", offers, err)
	}
}

func TestLocalNumberStripsCountryCode(t *testing.T) {
	if got := localNumber("628123456789", 62); got != "8123456789" {
		t.Fatalf("%s", got)
	}
	if got := localNumber("+62 0812-345", 62); got != "812345" {
		t.Fatalf("%s", got)
	}
}

func TestSplitUKNumber(t *testing.T) {
	code, local := SplitPhone("447529620432", 0)
	if code != 44 || local != "7529620432" {
		t.Fatalf("infer %d %s", code, local)
	}
	code, local = SplitPhone("447529620432", 44)
	if code != 44 || local != "7529620432" {
		t.Fatalf("hint %d %s", code, local)
	}
	n := Number{Phone: "447529620432", CountryCode: 16}
	applyPhoneSplit(&n)
	if n.PhoneCode != 44 || n.LocalNumber != "7529620432" {
		t.Fatalf("%+v", n)
	}
	n = Number{Phone: "447529620432", PhoneCode: 1, CountryCode: 16}
	applyPhoneSplit(&n)
	if n.PhoneCode != 44 || n.LocalNumber != "7529620432" {
		t.Fatalf("wrong +1 leftover %+v", n)
	}
	if CallingCodeForCountry(16) != 44 {
		t.Fatalf("uk country id")
	}
}

func TestParseAccessNumberUK(t *testing.T) {
	n, err := parseAccessNumber("ACCESS_NUMBER:123:447529620432", 16)
	if err != nil {
		t.Fatal(err)
	}
	if n.PhoneCode != 44 || n.LocalNumber != "7529620432" {
		t.Fatalf("%+v", n)
	}
}

func TestGetNumberV2JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "getNumberV2" {
			t.Fatalf("action=%s", r.URL.Query().Get("action"))
		}
		if r.URL.Query().Get("fixedPrice") != "true" || r.URL.Query().Get("maxPrice") != "0.15" {
			t.Fatalf("query %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{
			"activationId":"635468024",
			"phoneNumber":"628123456789",
			"activationCost":0.15,
			"countryCode":6,
			"countryPhoneCode":62
		}`)
	}))
	t.Cleanup(srv.Close)
	c := New("k", "ot")
	c.base = srv.URL
	n, err := c.GetNumber(6, 0.15)
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "635468024" || n.Phone != "628123456789" || n.LocalNumber != "8123456789" || n.PhoneCode != 62 {
		t.Fatalf("%+v", n)
	}
}

func TestStatusOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "STATUS_OK:123456")
	}))
	t.Cleanup(srv.Close)
	c := New("k", "ot")
	c.base = srv.URL
	code, wait, err := c.Status("1")
	if err != nil || wait || code != "123456" {
		t.Fatalf("%q %v %v", code, wait, err)
	}
}

func TestBalanceAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getBalance":
			_, _ = io.WriteString(w, "ACCESS_BALANCE:12.5")
		default:
			_, _ = io.WriteString(w, "NO_NUMBERS")
		}
	}))
	t.Cleanup(srv.Close)
	c := New("k", "ot")
	c.base = srv.URL
	n, err := c.Balance()
	if err != nil || n != 12.5 {
		t.Fatalf("%v %v", n, err)
	}
	_, err = c.GetNumber(6, 0.1)
	if err == nil || !strings.Contains(err.Error(), "没有号码") {
		t.Fatalf("%v", err)
	}
}
