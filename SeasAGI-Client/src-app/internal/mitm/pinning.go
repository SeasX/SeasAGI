package mitm

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"sync"
)

// PinStore manages certificate pins for known upstream domains.
type PinStore struct {
	mu   sync.RWMutex
	pins map[string][]string // domain → list of SHA-256 fingerprints
}

var defaultPinStore = &PinStore{
	pins: make(map[string][]string),
}

// SetPins sets the certificate pins for a domain.
func SetPins(domain string, fingerprints []string) {
	defaultPinStore.mu.Lock()
	defer defaultPinStore.mu.Unlock()
	defaultPinStore.pins[domain] = fingerprints
}

// CheckPin verifies that the peer certificate matches a known pin.
// Returns nil if no pin is set for the domain (pass-through), or if the cert matches.
// Returns an error if a pin is set but the cert does not match.
func CheckPin(domain string, state *tls.ConnectionState) error {
	defaultPinStore.mu.RLock()
	pins, ok := defaultPinStore.pins[domain]
	defaultPinStore.mu.RUnlock()

	if !ok || len(pins) == 0 {
		return nil // No pin set, allow
	}

	if state == nil || len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no peer certificate to verify pin for %s", domain)
	}

	cert := state.PeerCertificates[0]
	derBytes := cert.Raw
	hash := sha256.Sum256(derBytes)
	fingerprint := hex.EncodeToString(hash[:])

	for _, pin := range pins {
		if fingerprint == pin {
			return nil
		}
	}

	return fmt.Errorf("certificate pin mismatch for %s: got %s", domain, fingerprint)
}

// GetPins returns the pinned fingerprints for a domain.
func GetPins(domain string) []string {
	defaultPinStore.mu.RLock()
	defer defaultPinStore.mu.RUnlock()
	return defaultPinStore.pins[domain]
}

// ListPinnedDomains returns all domains with certificate pins.
func ListPinnedDomains() []string {
	defaultPinStore.mu.RLock()
	defer defaultPinStore.mu.RUnlock()
	domains := make([]string, 0, len(defaultPinStore.pins))
	for d := range defaultPinStore.pins {
		domains = append(domains, d)
	}
	return domains
}
