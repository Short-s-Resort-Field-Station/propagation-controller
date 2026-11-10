package species

import "testing"

func TestCatalogIDsUniqueAndComplete(t *testing.T) {
	seen := map[string]string{}
	for _, s := range Catalog {
		if len(s.ID) != 4 {
			t.Errorf("%s: id %q must be 4 letters", s.Common, s.ID)
		}
		if prev, dup := seen[s.ID]; dup {
			t.Errorf("duplicate id %s: %s and %s", s.ID, prev, s.Common)
		}
		seen[s.ID] = s.Common
		if s.Latin == "" || s.Common == "" || s.Category == "" {
			t.Errorf("%+v: missing field", s)
		}
		if s.Difficulty < 1 || s.Difficulty > 5 {
			t.Errorf("%s: difficulty %d", s.Common, s.Difficulty)
		}
	}
	for _, want := range []string{"Buttonbush", "Inland Sea Oats", "Live Oak"} {
		found := false
		for _, s := range Catalog {
			found = found || s.Common == want
		}
		if !found {
			t.Errorf("catalog missing %s", want)
		}
	}
}
