package datapagesgen

import (
	"crypto/rand"
	"datapages/app"
	"datapages/app/domain"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func BenchmarkHandlePageSearchGET(b *testing.B) {
	slugNonceGen := domain.NewSeededSlugNonceGenerator(1, 2)
	s := NewServer(app.NewApp(
		domain.NewRepository(nil, slugNonceGen)),
		WithAuthJWTConfig(AuthJWTConfig{
			Secret: []byte("okay"),
		}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/search?t=iphone&c=electronics&pmin=100&pmax=2000&l=zurich",
		nil,
	)
	w := httptest.NewRecorder()

	b.ResetTimer()

	for b.Loop() {
		w.Body.Reset()
		s.handlePageSearchGET(w, req)
	}
}

func BenchmarkCSRF(b *testing.B) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		b.Fatal(err)
	}

	s := NewServer(nil,
		WithAuthJWTConfig(AuthJWTConfig{Secret: []byte("secret")}),
		WithCSRFProtection(CSRFConfig{Secret: []byte("secret")}))

	userID := "user123"
	sessIssuedAt := time.Now()

	b.Run("Generate", func(b *testing.B) {
		var S string
		for b.Loop() {
			S = s.generateCSRFToken(userID, sessIssuedAt)
		}
		runtime.KeepAlive(S)
	})

	b.Run("Validate", func(b *testing.B) {
		token := s.generateCSRFToken(userID, sessIssuedAt)
		b.ResetTimer()
		for b.Loop() {
			if !s.validateCSRFToken(userID, sessIssuedAt, token) {
				b.Fatal("validation failed")
			}
		}
	})

	b.Run("GenerateAndValidate", func(b *testing.B) {
		for b.Loop() {
			token := s.generateCSRFToken(userID, sessIssuedAt)
			if !s.validateCSRFToken(userID, sessIssuedAt, token) {
				b.Fatal("validation failed")
			}
		}
	})

	b.Run("ValidateInvalid", func(b *testing.B) {
		invalidToken := "invalid-token-data"
		for b.Loop() {
			if s.validateCSRFToken(userID, sessIssuedAt, invalidToken) {
				b.Fatal("invalid token was accepted")
			}
		}
	})

	b.Run("ValidateWrongUser", func(b *testing.B) {
		token := s.generateCSRFToken(userID, sessIssuedAt)
		wrongUserID := "different-user"
		b.ResetTimer()
		for b.Loop() {
			if s.validateCSRFToken(wrongUserID, sessIssuedAt, token) {
				b.Fatal("token was accepted for wrong user")
			}
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				token := s.generateCSRFToken(userID, sessIssuedAt)
				if !s.validateCSRFToken(userID, sessIssuedAt, token) {
					b.Fatal("validation failed")
				}
			}
		})
	})
}
