package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	hash, err := HashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "rahasia123") {
		t.Error("password benar ditolak")
	}
	if CheckPassword(hash, "rahasia124") {
		t.Error("password salah diterima")
	}
	if _, err := HashPassword("pendek"); err != ErrPasswordLength {
		t.Errorf("password pendek: err = %v", err)
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Now()

	if !l.Allow("ip", now) || !l.Allow("ip", now) {
		t.Fatal("dua percobaan pertama harus diizinkan")
	}
	if l.Allow("ip", now) {
		t.Error("percobaan ketiga harus ditolak")
	}
	if !l.Allow("ip-lain", now) {
		t.Error("key lain tidak boleh ikut terblokir")
	}
	if !l.Allow("ip", now.Add(2*time.Minute)) {
		t.Error("setelah jendela waktu lewat harus diizinkan lagi")
	}
}
