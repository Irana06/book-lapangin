// Package auth menangani hash password, token session, dan pembatasan
// percobaan login.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8
	// bcrypt hanya memakai 72 byte pertama, jadi password lebih panjang ditolak.
	MaxPasswordLength = 72
)

var ErrPasswordLength = errors.New("password harus 8 sampai 72 karakter")

func HashPassword(password string) (string, error) {
	if len(password) < MinPasswordLength || len(password) > MaxPasswordLength {
		return "", ErrPasswordLength
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash dipakai saat email tidak terdaftar, supaya waktu respon login
// sama dan orang tidak bisa menebak email mana yang terdaftar.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func SpendCheckTime(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// NewSessionToken membuat token acak untuk cookie. Yang disimpan di database
// hanya hash-nya, jadi kebocoran database tidak membuat session bisa dipakai.
func NewSessionToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Limiter membatasi jumlah percobaan per key (misalnya IP) dalam satu jendela waktu.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: make(map[string][]time.Time)}
}

func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)

	// Cegah map tumbuh tanpa batas kalau ada banyak IP berbeda.
	if len(l.hits) > 10000 {
		l.hits = map[string][]time.Time{key: l.hits[key]}
	}
	return true
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
