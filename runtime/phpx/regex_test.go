package phpx

import "testing"

func TestPreg(t *testing.T) {
	if got := ToString(PregReplace("/(?=[A-Z][a-z])/", " ", "HumanSmaccerEntity")); got != " Human Smaccer Entity" {
		t.Errorf("lookahead replace: %q", got)
	}
	var m *Array
	if PregMatch("/^(\\w+)@(\\w+)\\.com$/", "bob@site.com", &m) != 1 || ToString(m.Get(2)) != "site" {
		t.Errorf("match groups: %v", m.Values())
	}
	if PregMatch("/(a)\\1/", "xaa", &m) != 1 {
		t.Error("backreference")
	}
	if got := Implode(",", PregSplit("/[\\s,]+/", "a, b  c,d")); got != "a,b,c,d" {
		t.Errorf("split: %q", got)
	}
	if got := ToString(PregReplace("/(\\d+)/", "[$1]", "a1b22")); got != "a[1]b[22]" {
		t.Errorf("replace: %q", got)
	}
	if got := ToString(PregReplace("/é(?=x)/u", "E", "éxé")); got != "Exé" {
		t.Errorf("unicode lookahead: %q", got)
	}
}
