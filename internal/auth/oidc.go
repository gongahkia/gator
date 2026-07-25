package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/norbot/internal/config"
)

type Principal struct {
	Subject string
	Groups  []string
}

type Validator struct {
	config config.OIDC
	client *http.Client
	mu     sync.Mutex
	keys   map[string]crypto.PublicKey
	expiry time.Time
}

func New(cfg config.OIDC) *Validator {
	return &Validator{config: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}
func (v *Validator) Enabled() bool { return v.config.Issuer != "" }

func (v *Validator) Validate(ctx context.Context, token string) (Principal, error) {
	if !v.Enabled() {
		return Principal{}, errors.New("oidc is not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, errors.New("malformed bearer token")
	}
	headerRaw, err := decode(parts[0])
	if err != nil {
		return Principal{}, err
	}
	claimsRaw, err := decode(parts[1])
	if err != nil {
		return Principal{}, err
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return Principal{}, fmt.Errorf("decode jwt header: %w", err)
	}
	if header.KeyID == "" || (header.Algorithm != "RS256" && header.Algorithm != "RS384" && header.Algorithm != "RS512" && header.Algorithm != "ES256" && header.Algorithm != "ES384" && header.Algorithm != "ES512") {
		return Principal{}, errors.New("unsupported jwt signing algorithm")
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return Principal{}, fmt.Errorf("decode jwt claims: %w", err)
	}
	if err := v.validateClaims(claims); err != nil {
		return Principal{}, err
	}
	key, err := v.key(ctx, header.KeyID)
	if err != nil {
		return Principal{}, err
	}
	signature, err := decode(parts[2])
	if err != nil {
		return Principal{}, err
	}
	if err := verify(header.Algorithm, key, []byte(parts[0]+"."+parts[1]), signature); err != nil {
		return Principal{}, err
	}
	groups := stringsSlice(claims[v.config.GroupsClaim])
	if !intersects(groups, v.config.OperatorGroups) {
		return Principal{}, errors.New("operator group is required")
	}
	subject, _ := claims["sub"].(string)
	if subject == "" {
		return Principal{}, errors.New("jwt subject is required")
	}
	return Principal{Subject: subject, Groups: groups}, nil
}

func (v *Validator) validateClaims(claims map[string]any) error {
	issuer, _ := claims["iss"].(string)
	if issuer != v.config.Issuer {
		return errors.New("jwt issuer mismatch")
	}
	if !contains(stringsSlice(claims["aud"]), v.config.Audience) {
		return errors.New("jwt audience mismatch")
	}
	exp, ok := number(claims["exp"])
	if !ok || time.Now().UTC().Unix() >= exp {
		return errors.New("jwt is expired")
	}
	if nbf, ok := number(claims["nbf"]); ok && time.Now().UTC().Unix() < nbf {
		return errors.New("jwt is not active")
	}
	return nil
}

func (v *Validator) key(ctx context.Context, id string) (crypto.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.expiry) {
		if key := v.keys[id]; key != nil {
			return key, nil
		}
	}
	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.keys, v.expiry = keys, time.Now().Add(15*time.Minute)
	key := keys[id]
	if key == nil {
		return nil, errors.New("jwt key id is unknown")
	}
	return key, nil
}

func (v *Validator) fetchKeys(ctx context.Context) (map[string]crypto.PublicKey, error) {
	issuer := strings.TrimRight(v.config.Issuer, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	response, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery status %d", response.StatusCode)
	}
	var discovery struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&discovery); err != nil || discovery.JWKSURI == "" {
		return nil, errors.New("invalid oidc discovery response")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, discovery.JWKSURI, nil)
	if err != nil {
		return nil, err
	}
	response, err = v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks status %d", response.StatusCode)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	keys := make(map[string]crypto.PublicKey, len(set.Keys))
	for _, value := range set.Keys {
		key, err := value.publicKey()
		if err != nil {
			return nil, err
		}
		keys[value.ID] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks has no keys")
	}
	return keys, nil
}

type jwk struct {
	ID    string `json:"kid"`
	Type  string `json:"kty"`
	Curve string `json:"crv"`
	N     string `json:"n"`
	E     string `json:"e"`
	X     string `json:"x"`
	Y     string `json:"y"`
}

func (j jwk) publicKey() (crypto.PublicKey, error) {
	if j.ID == "" {
		return nil, errors.New("jwk kid is required")
	}
	if j.Type == "RSA" {
		n, err := decode(j.N)
		if err != nil {
			return nil, err
		}
		e, err := decode(j.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("invalid rsa exponent")
		}
		exponent := 0
		for _, value := range e {
			exponent = exponent<<8 | int(value)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}, nil
	}
	if j.Type == "EC" {
		curve := map[string]elliptic.Curve{"P-256": elliptic.P256(), "P-384": elliptic.P384(), "P-521": elliptic.P521()}[j.Curve]
		if curve == nil {
			return nil, errors.New("unsupported ec curve")
		}
		x, err := decode(j.X)
		if err != nil {
			return nil, err
		}
		y, err := decode(j.Y)
		if err != nil {
			return nil, err
		}
		key := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !curve.IsOnCurve(key.X, key.Y) {
			return nil, errors.New("invalid ec key")
		}
		return key, nil
	}
	return nil, errors.New("unsupported jwk type")
}

func verify(algorithm string, key crypto.PublicKey, message, signature []byte) error {
	bits := 256
	if strings.HasSuffix(algorithm, "384") {
		bits = 384
	}
	if strings.HasSuffix(algorithm, "512") {
		bits = 512
	}
	var digest []byte
	var hash crypto.Hash
	switch bits {
	case 256:
		value := sha256.Sum256(message)
		digest = value[:]
		hash = crypto.SHA256
	case 384:
		value := sha512.Sum384(message)
		digest = value[:]
		hash = crypto.SHA384
	case 512:
		value := sha512.Sum512(message)
		digest = value[:]
		hash = crypto.SHA512
	}
	if rsaKey, ok := key.(*rsa.PublicKey); ok && strings.HasPrefix(algorithm, "RS") {
		if err := rsa.VerifyPKCS1v15(rsaKey, hash, digest, signature); err != nil {
			return errors.New("invalid jwt signature")
		}
		return nil
	}
	if ecKey, ok := key.(*ecdsa.PublicKey); ok && strings.HasPrefix(algorithm, "ES") {
		size := (ecKey.Curve.Params().BitSize + 7) / 8
		if len(signature) != 2*size {
			return errors.New("invalid ecdsa signature length")
		}
		if !ecdsa.Verify(ecKey, digest, new(big.Int).SetBytes(signature[:size]), new(big.Int).SetBytes(signature[size:])) {
			return errors.New("invalid jwt signature")
		}
		return nil
	}
	return errors.New("jwt key and algorithm mismatch")
}
func decode(value string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(value) }
func number(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), v >= 0
	case json.Number:
		n, e := v.Int64()
		return n, e == nil
	default:
		return 0, false
	}
}
func stringsSlice(value any) []string {
	if value == nil {
		return nil
	}
	if item, ok := value.(string); ok {
		return []string{item}
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	return values
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func intersects(left, right []string) bool {
	for _, value := range left {
		if contains(right, value) {
			return true
		}
	}
	return false
}
