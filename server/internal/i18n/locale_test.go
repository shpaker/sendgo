package i18n

import "testing"

func TestNegotiator_Negotiate(t *testing.T) {
	n := NewNegotiator([]string{"en-US", "ru", "de"})

	cases := map[string]string{
		"":                                    "en-US",
		"ru-RU,ru;q=0.9":                      "ru",
		"de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7": "de",
		"fr-FR,fr;q=0.9,en;q=0.8":             "en-US", // fr is not in available; matches en
		"xx":                                  "en-US", // invalid → default
	}
	for input, want := range cases {
		got := n.Negotiate(input)
		if got != want {
			t.Errorf("Negotiate(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNegotiator_EmptyList(t *testing.T) {
	n := NewNegotiator(nil)
	if got := n.Negotiate("ru-RU"); got != "en-US" {
		t.Errorf("empty available list should fall back to en-US, got %q", got)
	}
}
