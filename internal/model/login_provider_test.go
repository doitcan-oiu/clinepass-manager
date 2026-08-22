package model

import "testing"

func TestNormalizeLoginProvider(t *testing.T) {
	cases := map[string]string{
		"":          LoginGoogle,
		"google":    LoginGoogle,
		"GOOGLE":    LoginGoogle,
		"microsoft": LoginMicrosoft,
		"MS":        LoginMicrosoft,
		"outlook":   LoginMicrosoft,
		"hotmail":   LoginMicrosoft,
		"live":      LoginMicrosoft,
	}
	for in, want := range cases {
		if got := NormalizeLoginProvider(in); got != want {
			t.Fatalf("%q => %q want %q", in, got, want)
		}
	}
}
