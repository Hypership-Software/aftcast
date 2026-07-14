package analytics

import (
	"testing"

	"github.com/Hypership-Software/atlas/internal/schema"
)

func TestShippingProfileEligibilityAndRate(t *testing.T) {
	sessions := []SessionStat{
		{CaptureVersion: 1, FilesChanged: 2},
		{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 0},
		{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 2, Shipped: true},
		{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 1, Shipped: false},
		{CaptureVersion: schema.DeliverySignalVersion, FilesChanged: 0, Shipped: true},
	}

	got := ShippingProfile(sessions)
	if got.Eligible != 3 || got.Shipped != 2 || got.Rate != 2.0/3.0 {
		t.Fatalf("ShippingProfile = %+v, want {eligible:3 shipped:2 rate:2/3}", got)
	}
}

func TestShippingProfileEmptyIsZero(t *testing.T) {
	got := ShippingProfile(nil)
	if got != (ShippedProfile{}) {
		t.Fatalf("empty profile = %+v", got)
	}
}
