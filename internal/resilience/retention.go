package resilience

import (
	"sync"
	"time"
)

// RetentionPolicy defines how long various data categories are kept.
type RetentionPolicy struct {
	AuditLogDays        int  `json:"audit_log_days" yaml:"audit_log_days"`
	EvidenceDays        int  `json:"evidence_days" yaml:"evidence_days"`
	ApprovalHistoryDays int  `json:"approval_history_days" yaml:"approval_history_days"`
	CompressAfterDays   int  `json:"compress_after_days" yaml:"compress_after_days"`
	AutoCleanup         bool `json:"auto_cleanup" yaml:"auto_cleanup"`
}

// RetentionReport summarises what a cleanup cycle removed.
type RetentionReport struct {
	AuditEntriesRemoved    int       `json:"audit_entries_removed"`
	EvidenceRecordsRemoved int       `json:"evidence_records_removed"`
	ApprovalsRemoved       int       `json:"approvals_removed"`
	CompressedRecords      int       `json:"compressed_records"`
	CleanedAt              time.Time `json:"cleaned_at"`
}

// RetentionStats provides a point-in-time view of stored data volumes.
type RetentionStats struct {
	AuditEntries       int       `json:"audit_entries"`
	EvidenceRecords    int       `json:"evidence_records"`
	ApprovalHistory    int       `json:"approval_history"`
	OldestAuditEntry   time.Time `json:"oldest_audit_entry"`
	OldestEvidence     time.Time `json:"oldest_evidence"`
	OldestApproval     time.Time `json:"oldest_approval"`
	PolicyDays         int       `json:"policy_days"`
	AutoCleanupEnabled bool      `json:"auto_cleanup_enabled"`
}

// timestampedRecord is a generic record with a timestamp for retention
// tracking in the in-memory store.
type timestampedRecord struct {
	Category  string    // "audit", "evidence", "approval"
	CreatedAt time.Time
}

// RetentionManager enforces retention policies on stored data.
type RetentionManager struct {
	mu      sync.Mutex
	policy  RetentionPolicy
	records []timestampedRecord
}

// NewRetentionManager creates a RetentionManager with the given policy.
func NewRetentionManager(policy RetentionPolicy) *RetentionManager {
	return &RetentionManager{
		policy:  policy,
		records: make([]timestampedRecord, 0),
	}
}

// AddRecord registers a record for retention tracking (in-memory).
func (m *RetentionManager) AddRecord(category string, createdAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, timestampedRecord{Category: category, CreatedAt: createdAt})
}

// Cleanup removes records that exceed the retention policy and returns a report.
func (m *RetentionManager) Cleanup() RetentionReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	report := RetentionReport{CleanedAt: now}

	kept := make([]timestampedRecord, 0, len(m.records))
	for _, rec := range m.records {
		age := int(now.Sub(rec.CreatedAt).Hours() / 24)
		switch rec.Category {
		case "audit":
			if m.policy.AuditLogDays > 0 && age > m.policy.AuditLogDays {
				report.AuditEntriesRemoved++
				continue
			}
		case "evidence":
			if m.policy.EvidenceDays > 0 && age > m.policy.EvidenceDays {
				report.EvidenceRecordsRemoved++
				continue
			}
		case "approval":
			if m.policy.ApprovalHistoryDays > 0 && age > m.policy.ApprovalHistoryDays {
				report.ApprovalsRemoved++
				continue
			}
		}
		// Count records eligible for compression.
		if m.policy.CompressAfterDays > 0 && age > m.policy.CompressAfterDays {
			report.CompressedRecords++
		}
		kept = append(kept, rec)
	}
	m.records = kept
	return report
}

// Stats returns current retention statistics.
func (m *RetentionManager) Stats() RetentionStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := RetentionStats{
		PolicyDays:         m.policy.AuditLogDays,
		AutoCleanupEnabled: m.policy.AutoCleanup,
	}
	for _, rec := range m.records {
		switch rec.Category {
		case "audit":
			stats.AuditEntries++
			if stats.OldestAuditEntry.IsZero() || rec.CreatedAt.Before(stats.OldestAuditEntry) {
				stats.OldestAuditEntry = rec.CreatedAt
			}
		case "evidence":
			stats.EvidenceRecords++
			if stats.OldestEvidence.IsZero() || rec.CreatedAt.Before(stats.OldestEvidence) {
				stats.OldestEvidence = rec.CreatedAt
			}
		case "approval":
			stats.ApprovalHistory++
			if stats.OldestApproval.IsZero() || rec.CreatedAt.Before(stats.OldestApproval) {
				stats.OldestApproval = rec.CreatedAt
			}
		}
	}
	return stats
}
