package app

import "testing"

func TestPizzaPricing(t *testing.T) {
	c := PizzaConfiguration{Enabled: true, Policy: "weighted_average", MaxFlavors: 4, Flavors: []PizzaFlavor{{"a", "Calabresa", 3001, true}, {"b", "Queijo", 4002, true}, {"c", "Indisponível", 9999, false}}}
	for _, tc := range []struct {
		name, policy string
		parts        []PizzaPart
		want         int64
		invalid      bool
	}{
		{"half-cent", "weighted_average", []PizzaPart{{"a", 1}, {"b", 1}}, 3502, false},
		{"weighted-thirds", "weighted_average", []PizzaPart{{"a", 2}, {"b", 1}}, 3335, false},
		{"highest-ignores-proportion", "highest", []PizzaPart{{"a", 7}, {"b", 1}}, 4002, false},
		{"single", "weighted_average", []PizzaPart{{"a", 1}}, 3001, false},
		{"inactive", "highest", []PizzaPart{{"c", 1}}, 0, true},
		{"unknown", "highest", []PizzaPart{{"other-store", 1}}, 0, true},
		{"duplicate", "highest", []PizzaPart{{"a", 1}, {"a", 1}}, 0, true},
		{"zero-parts", "highest", []PizzaPart{{"a", 0}}, 0, true},
		{"negative", "highest", []PizzaPart{{"a", -1}}, 0, true},
		{"over-limit", "highest", []PizzaPart{{"a", 7}, {"b", 2}}, 0, true},
		{"missing", "highest", nil, 0, true},
		{"unknown-policy", "forged", []PizzaPart{{"a", 1}}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.Policy = tc.policy
			s, err := pizzaPrice(c, tc.parts)
			if tc.invalid {
				if err == nil {
					t.Fatal("accepted invalid selection")
				}
				return
			}
			if err != nil || s.Price != tc.want {
				t.Fatalf("got %v %v, want %d", s, err, tc.want)
			}
		})
	}
	c.Enabled = false
	if _, err := pizzaPrice(c, []PizzaPart{{"a", 1}}); err == nil {
		t.Fatal("flavors accepted on ordinary product")
	}
	c.Enabled = true
	c.Policy = "highest"
	c.MaxFlavors = 1
	if _, err := pizzaPrice(c, []PizzaPart{{"a", 1}, {"b", 1}}); err == nil {
		t.Fatal("flavor limit bypassed")
	}
}
