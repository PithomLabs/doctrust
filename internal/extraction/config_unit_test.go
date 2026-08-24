package extraction

import "testing"

func TestParseCurrencyValue_StripsUnits(t *testing.T) {
	cases := []struct{ in, want string }{
		{"4,650.00 KG", "4650"},
		{"5,150.00 kg", "5150"},
		{"$120,000", "120000"},
		{"24.000 CBM", "24"},
	}
	for _, c := range cases {
		got := ParseCurrencyValue(c.in)
		f, ok := got.(float64)
		if !ok {
			t.Fatalf("%q: expected float64, got %T (%v)", c.in, got, got)
		}
		if got2 := ParseCurrencyValue(c.want); f != got2.(float64) {
			t.Fatalf("%q: parsed %v, want %s", c.in, f, c.want)
		}
	}
	if ParseCurrencyValue("N/A") != "N/A" {
		t.Fatal("non-numeric strings must pass through unchanged")
	}
}
