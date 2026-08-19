// Package auth stores and retrieves the GitHub token. The token is never
// written to disk in plaintext: the OS keychain is the default, and
// DEADWOOD_GITHUB_TOKEN is the explicit opt-in for environments without one.
package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	// Service is the keychain service name.
	Service = "deadwood"
	// Account is the keychain account that holds the GitHub token.
	Account = "github-token"
	// EnvToken is the environment variable checked before the keychain.
	EnvToken = "DEADWOOD_GITHUB_TOKEN"
)

// ErrNotFound reports that no token is available from the env var or the
// keychain.
var ErrNotFound = errors.New("no GitHub token found")

// Ring is the keychain operations auth needs. Production uses the OS
// keychain; tests use an in-memory implementation so they never touch the
// real one.
type Ring interface {
	// Get returns the stored secret, or keyring.ErrNotFound if absent.
	Get(service, user string) (string, error)
	// Set writes the secret.
	Set(service, user, password string) error
	// Delete removes the secret.
	Delete(service, user string) error
}

// Token is a retrieved credential. FromEnv is true when DEADWOOD_GITHUB_TOKEN
// supplied it, so callers can warn that this is less secure than the keychain.
type Token struct {
	Value   string
	FromEnv bool
}

// Store looks up and persists tokens. Get checks the env var before the
// keychain, matching spec Section 7.1.
type Store struct {
	ring      Ring
	lookupEnv func(string) (string, bool)
}

// DefaultStore uses the OS keychain and the process environment.
func DefaultStore() *Store {
	return NewStore(osRing{}, os.LookupEnv)
}

// NewStore builds a Store. lookupEnv may be nil, in which case the process
// environment is used.
func NewStore(ring Ring, lookupEnv func(string) (string, bool)) *Store {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	return &Store{ring: ring, lookupEnv: lookupEnv}
}

// Get returns the token. An empty env var is ignored so a shell that exports
// DEADWOOD_GITHUB_TOKEN= does not mask a valid keychain entry.
func (s *Store) Get() (Token, error) {
	if value, ok := s.lookupEnv(EnvToken); ok && value != "" {
		return Token{Value: value, FromEnv: true}, nil
	}

	value, err := s.ring.Get(Service, Account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return Token{}, ErrNotFound
		}
		return Token{}, fmt.Errorf("reading GitHub token from keychain: %w", err)
	}
	if value == "" {
		return Token{}, ErrNotFound
	}
	return Token{Value: value}, nil
}

// Set writes the token to the keychain. It never writes to the environment.
func (s *Store) Set(token string) error {
	if token == "" {
		return errors.New("refusing to store an empty GitHub token")
	}
	if err := s.ring.Set(Service, Account, token); err != nil {
		return fmt.Errorf("storing GitHub token in keychain: %w", err)
	}
	return nil
}

// Delete removes the keychain entry. A missing entry is not an error. The
// environment variable, if set, is left alone — the user owns that.
func (s *Store) Delete() error {
	err := s.ring.Delete(Service, Account)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("deleting GitHub token from keychain: %w", err)
}

// osRing is the production keychain.
type osRing struct{}

func (osRing) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (osRing) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (osRing) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// MemoryRing is an in-memory keychain for tests. It is not used in production.
type MemoryRing struct {
	values map[string]string
}

// NewMemoryRing returns an empty in-memory ring.
func NewMemoryRing() *MemoryRing {
	return &MemoryRing{values: make(map[string]string)}
}

func (m *MemoryRing) key(service, user string) string { return service + "\x00" + user }

// Get implements Ring.
func (m *MemoryRing) Get(service, user string) (string, error) {
	value, ok := m.values[m.key(service, user)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

// Set implements Ring.
func (m *MemoryRing) Set(service, user, password string) error {
	m.values[m.key(service, user)] = password
	return nil
}

// Delete implements Ring.
func (m *MemoryRing) Delete(service, user string) error {
	k := m.key(service, user)
	if _, ok := m.values[k]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.values, k)
	return nil
}
