package server

import (
	"testing"
	"time"
)

func TestReportReplayWindowBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if !reportWithinReplayWindow(now, now) {
		t.Fatal("current report was rejected")
	}
	if !reportWithinReplayWindow(now.Add(-reportReplayWindow), now) {
		t.Fatal("report exactly at replay cutoff was rejected")
	}
	if reportWithinReplayWindow(now.Add(-reportReplayWindow-time.Second), now) {
		t.Fatal("report outside replay window was accepted")
	}
}

func TestReportIDValidatorAcceptsOnlyV4AndV7UUIDs(t *testing.T) {
	for _, id := range []string{
		"00000000-0000-4000-8000-000000000001",
		"0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
	} {
		if !validUUIDString(id) {
			t.Errorf("validUUIDString(%q) rejected compatible UUID", id)
		}
	}
	for _, id := range []string{
		"00000000-0000-1000-8000-000000000001",
		"00000000-0000-7000-7000-000000000001",
	} {
		if validUUIDString(id) {
			t.Errorf("validUUIDString(%q) accepted unsupported UUID version/variant", id)
		}
	}
}
