package analytics

import (
	"testing"
	"time"

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

func TestShippingProfileTracksFirstV2SessionBeforeEligibility(t *testing.T) {
	sessions := []SessionStat{
		{CaptureVersion: 1, Started: "2026-07-10T09:00:00Z", FilesChanged: 2},
		{CaptureVersion: schema.DeliverySignalVersion, Started: "2026-07-15T11:00:00Z"},
		{CaptureVersion: schema.DeliverySignalVersion, Started: "not-a-time", FilesChanged: 1},
		{CaptureVersion: schema.DeliverySignalVersion, Started: "2026-07-16T11:00:00Z", FilesChanged: 1},
	}
	got := ShippingProfile(sessions)
	want := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)
	if !got.TrackingSince.Equal(want) {
		t.Fatalf("TrackingSince = %v, want %v", got.TrackingSince, want)
	}
	if got.Eligible != 2 {
		t.Fatalf("Eligible = %d, want 2; tracking must not change eligibility", got.Eligible)
	}
}
