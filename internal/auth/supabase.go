package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	keyCacheTTL      = time.Hour
	minRefetchPeriod = 30 * time.Second
	clockSkew        = 30 * time.Second
)

type Verifier struct {
	issuer  string
	jwksURL string
	client  *http.Client

	mu      sync.Mutex
	keys    map[string]crypto.PublicKey
	fetched time.Time
}

type Claims struct {
	Sub      string
	Provider string
	Email    string
	Name     string
}

func NewVerifier(supabaseURL string) *Verifier {
	base := strings.TrimRight(supabaseURL, "/")
	return &Verifier{
		issuer:  base + "/auth/v1",
		jwksURL: base + "/auth/v1/.well-known/jwks.json",
		client:  &http.Client{Timeout: 10 * time.Second},
		keys:    map[string]crypto.PublicKey{},
	}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtPayload struct {
	Sub         string          `json:"sub"`
	Iss         string          `json:"iss"`
	Aud         json.RawMessage `json:"aud"`
	Exp         float64         `json:"exp"`
	Email       string          `json:"email"`
	AppMetadata struct {
		Provider string `json:"provider"`
	} `json:"app_metadata"`
	UserMetadata struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
	} `json:"user_metadata"`
}

func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("malformed token header")
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("malformed token header")
	}
	if header.Alg != "ES256" && header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported token algorithm %q", header.Alg)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("malformed token signature")
	}

	key, err := v.keyFor(ctx, header.Kid)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := verifySignature(header.Alg, key, hash[:], sig); err != nil {
		return nil, err
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("malformed token payload")
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("malformed token payload")
	}

	if payload.Iss != v.issuer {
		return nil, errors.New("token issuer mismatch")
	}
	if !audienceHas(payload.Aud, "authenticated") {
		return nil, errors.New("token audience mismatch")
	}
	if time.Now().After(time.Unix(int64(payload.Exp), 0).Add(clockSkew)) {
		return nil, errors.New("token expired")
	}
	if payload.Sub == "" {
		return nil, errors.New("token has no subject")
	}

	name := payload.UserMetadata.FullName
	if name == "" {
		name = payload.UserMetadata.Name
	}

	return &Claims{
		Sub:      payload.Sub,
		Provider: payload.AppMetadata.Provider,
		Email:    payload.Email,
		Name:     name,
	}, nil
}

func audienceHas(raw json.RawMessage, want string) bool {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == want
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}

func verifySignature(alg string, key crypto.PublicKey, hash, sig []byte) error {
	switch alg {
	case "ES256":
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("token key type does not match algorithm")
		}
		if len(sig) != 64 {
			return errors.New("invalid token signature length")
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(pub, hash, r, s) {
			return errors.New("invalid token signature")
		}
		return nil
	case "RS256":
		pub, ok := key.(*rsa.PublicKey)
		if !ok {
			return errors.New("token key type does not match algorithm")
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash, sig); err != nil {
			return errors.New("invalid token signature")
		}
		return nil
	}
	return errors.New("unsupported token algorithm")
}

func (v *Verifier) keyFor(ctx context.Context, kid string) (crypto.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if key, ok := v.keys[kid]; ok && time.Since(v.fetched) < keyCacheTTL {
		return key, nil
	}

	if time.Since(v.fetched) >= minRefetchPeriod || len(v.keys) == 0 {
		if err := v.refresh(ctx); err != nil {
			if key, ok := v.keys[kid]; ok {
				return key, nil
			}
			return nil, err
		}
	}

	key, ok := v.keys[kid]
	if !ok {
		return nil, errors.New("unknown token signing key")
	}
	return key, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("building jwks request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching jwks: status %d", resp.StatusCode)
	}

	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decoding jwks: %w", err)
	}

	keys := make(map[string]crypto.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		switch k.Kty {
		case "EC":
			if k.Crv != "P-256" {
				continue
			}
			x, errX := base64.RawURLEncoding.DecodeString(k.X)
			y, errY := base64.RawURLEncoding.DecodeString(k.Y)
			if errX != nil || errY != nil {
				continue
			}
			keys[k.Kid] = &ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     new(big.Int).SetBytes(x),
				Y:     new(big.Int).SetBytes(y),
			}
		case "RSA":
			n, errN := base64.RawURLEncoding.DecodeString(k.N)
			e, errE := base64.RawURLEncoding.DecodeString(k.E)
			if errN != nil || errE != nil {
				continue
			}
			keys[k.Kid] = &rsa.PublicKey{
				N: new(big.Int).SetBytes(n),
				E: int(new(big.Int).SetBytes(e).Int64()),
			}
		}
	}
	if len(keys) == 0 {
		return errors.New("jwks contained no usable keys")
	}

	v.keys = keys
	v.fetched = time.Now()
	return nil
}
