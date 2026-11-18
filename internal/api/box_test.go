package api

import "testing"

func TestParseWireless(t *testing.T) {
	table := `Inter-| sta-|   Quality        |   Discarded packets               | Missed | WE
 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
wlx9cefd5f72bcb: 0000   49.  -61.  -256        0      0      0      0      0        0
`
	dbm, pct := parseWireless(table, "wlx9cefd5f72bcb")
	if dbm == nil || *dbm != -61 {
		t.Fatalf("dbm = %v, want -61", dbm)
	}
	if pct == nil || *pct != 70 {
		t.Fatalf("percent = %v, want 70", pct)
	}
	if d, p := parseWireless(table, "eno1"); d != nil || p != nil {
		t.Fatalf("an interface not in the table has no signal, got %v %v", d, p)
	}
}
