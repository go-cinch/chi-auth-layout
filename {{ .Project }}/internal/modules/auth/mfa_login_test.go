package auth

import (
	"testing"
	"time"
)

func TestConfiguredMFALoginTicketLifetime(t *testing.T) {
	m, _, ctx, identity, state := newVerificationTest(t)
	m.verification.config.LoginTicketTTL = 37500 * time.Millisecond
	started := time.Now()
	result, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.ttls[loginTicketPrefix+result.MFATicket] != m.verification.config.LoginTicketTTL || result.ExpiredAt < started.Add(m.verification.config.LoginTicketTTL).UnixMilli() || result.ExpiredAt > time.Now().Add(m.verification.config.LoginTicketTTL).UnixMilli() {
		t.Fatal("ticket deadline or Redis TTL ignored configuration")
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Nanosecond} {
		cfg := m.verification.config
		cfg.LoginTicketTTL = ttl
		if _, err := NewVerification(m.store, state, cfg); err == nil {
			t.Fatal("invalid ticket TTL accepted", ttl)
		}
	}
}
