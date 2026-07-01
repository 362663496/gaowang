package services

import "testing"

func Test_PasswordMatches_returns_true_when_password_matches_hash(t *testing.T) {
	// Given
	password := "correct-horse-battery-staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	// When
	matches := PasswordMatches(hash, password)

	// Then
	if !matches {
		t.Fatal("PasswordMatches() = false, want true")
	}
}

func Test_PasswordMatches_returns_false_when_password_does_not_match_hash(t *testing.T) {
	// Given
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	// When
	matches := PasswordMatches(hash, "wrong-password")

	// Then
	if matches {
		t.Fatal("PasswordMatches() = true, want false")
	}
}
