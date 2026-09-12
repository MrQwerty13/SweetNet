package auth

import "testing"

func TestPasswordAndSecret(t *testing.T) {
	h, err := HashPassword("a very good password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword("a very good password", h) || CheckPassword("wrong password", h) {
		t.Fatal("password verification")
	}
	h2, _ := HashPassword("a very good password")
	if h == h2 {
		t.Fatal("salt not random")
	}
	a, _ := Secret()
	b, _ := Secret()
	if a == b || len(a) < 43 || HashSecret(a) == a {
		t.Fatal("secret generation")
	}
	if _, ok := Username("Owner"); !ok {
		t.Fatal("normalization")
	}
	if _, ok := Username("admin injected"); ok {
		t.Fatal("invalid username accepted")
	}
	if ValidPassword("short") || ValidName(" ") {
		t.Fatal("validation")
	}
}
func BenchmarkHashPassword(b *testing.B) {
	for b.Loop() {
		if _, err := HashPassword("benchmark-password-only"); err != nil {
			b.Fatal(err)
		}
	}
}
