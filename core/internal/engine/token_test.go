package engine

import "testing"

func TestExplicitTokenRequiresMinimumLength(t *testing.T) {
	if _, _, err := LoadOrCreateToken("too-short"); err == nil {
		t.Fatal("expected short explicit token to be rejected")
	}
	value := "0123456789abcdef0123456789abcdef"
	token, source, err := LoadOrCreateToken(value)
	if err != nil {
		t.Fatal(err)
	}
	if token != value || source != "environment" {
		t.Fatalf("unexpected token result source=%q", source)
	}
}
