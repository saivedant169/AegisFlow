package integrations

import "github.com/saivedant169/AegisFlow/internal/approval"

// ApprovalNotifier sends approval lifecycle notifications to external systems.
type ApprovalNotifier interface {
	// NotifyReview posts a new approval request for human review.
	NotifyReview(item *approval.ApprovalItem) error

	// NotifyApproved posts a follow-up that the item was approved.
	NotifyApproved(item *approval.ApprovalItem) error

	// NotifyDenied posts a follow-up that the item was denied.
	NotifyDenied(item *approval.ApprovalItem) error
}
