package resilience

import (
	"testing"
	"time"
)

func TestRetentionManager_Cleanup(t *testing.T) {
	policy := RetentionPolicy{
		AuditLogDays:        30,
		EvidenceDays:        90,
		ApprovalHistoryDays: 60,
		CompressAfterDays:   7,
		AutoCleanup:         true,
	}
	rm := NewRetentionManager(policy)

	now := time.Now().UTC()
	// Old records that should be cleaned up.
	rm.AddRecord("audit", now.AddDate(0, 0, -60))     // 60 days old > 30 day policy
	rm.AddRecord("evidence", now.AddDate(0, 0, -100)) // 100 days old > 90 day policy
	rm.AddRecord("approval", now.AddDate(0, 0, -70))  // 70 days old > 60 day policy
	// Recent records that should be kept.
	rm.AddRecord("audit", now.AddDate(0, 0, -5))
	rm.AddRecord("evidence", now.AddDate(0, 0, -10))
	rm.AddRecord("approval", now.AddDate(0, 0, -3))

	report := rm.Cleanup()
	if report.AuditEntriesRemoved != 1 {
		t.Fatalf("expected 1 audit removed, got %d", report.AuditEntriesRemoved)
	}
	if report.EvidenceRecordsRemoved != 1 {
		t.Fatalf("expected 1 evidence removed, got %d", report.EvidenceRecordsRemoved)
	}
	if report.ApprovalsRemoved != 1 {
		t.Fatalf("expected 1 approval removed, got %d", report.ApprovalsRemoved)
	}
}

func TestRetentionManager_Stats(t *testing.T) {
	policy := RetentionPolicy{
		AuditLogDays: 90,
		AutoCleanup:  true,
	}
	rm := NewRetentionManager(policy)

	now := time.Now().UTC()
	rm.AddRecord("audit", now.AddDate(0, 0, -10))
	rm.AddRecord("audit", now.AddDate(0, 0, -5))
	rm.AddRecord("evidence", now.AddDate(0, 0, -3))

	stats := rm.Stats()
	if stats.AuditEntries != 2 {
		t.Fatalf("expected 2 audit entries, got %d", stats.AuditEntries)
	}
	if stats.EvidenceRecords != 1 {
		t.Fatalf("expected 1 evidence record, got %d", stats.EvidenceRecords)
	}
	if stats.PolicyDays != 90 {
		t.Fatalf("expected policy days 90, got %d", stats.PolicyDays)
	}
	if !stats.AutoCleanupEnabled {
		t.Fatal("expected auto_cleanup enabled")
	}
}

func TestRetentionManager_CompressAfterDays(t *testing.T) {
	policy := RetentionPolicy{
		AuditLogDays:      365,
		CompressAfterDays: 7,
	}
	rm := NewRetentionManager(policy)

	now := time.Now().UTC()
	rm.AddRecord("audit", now.AddDate(0, 0, -10)) // older than 7 days
	rm.AddRecord("audit", now.AddDate(0, 0, -1))  // recent

	report := rm.Cleanup()
	if report.CompressedRecords != 1 {
		t.Fatalf("expected 1 compressed, got %d", report.CompressedRecords)
	}
	if report.AuditEntriesRemoved != 0 {
		t.Fatalf("expected 0 removed (within 365 day policy), got %d", report.AuditEntriesRemoved)
	}
}
