package main

import (
	"testing"
)

func TestServiceMapping(t *testing.T) {
	got, err := serviceMapping("api/order-service-proto/user.proto", "example.com/consumer")
	if err != nil || got != "api/order-service-proto/user.proto=example.com/consumer/api/order-service;order_service" {
		t.Fatalf("mapping: %s %v", got, err)
	}
	for _, source := range []string{"api/user.proto", "api/user/user.proto", "api/-proto/user.proto", "api/123-proto/user.proto"} {
		if _, err := serviceMapping(source, "example.com/consumer"); err == nil {
			t.Fatalf("invalid path accepted: %s", source)
		}
	}
}
