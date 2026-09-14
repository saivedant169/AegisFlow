package approval

import (
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// AdminAdapter bridges the Queue to the admin API.
type AdminAdapter struct {
	queue  *Queue
	tenant string
	scoped bool
}

func NewAdminAdapter(q *Queue) *AdminAdapter {
	return &AdminAdapter{queue: q}
}

func (a *AdminAdapter) Pending() any {
	return a.filter(a.queue.Pending())
}

func (a *AdminAdapter) History(limit int) any {
	items := a.filter(a.queue.History(0))
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func (a *AdminAdapter) Get(id string) (any, error) {
	item, err := a.queue.Get(id)
	if err != nil || !a.visible(item) {
		return nil, fmt.Errorf("approval not found")
	}
	return item, nil
}

func (a *AdminAdapter) Approve(id, reviewer, comment string) (any, error) {
	if _, err := a.Get(id); err != nil {
		return nil, err
	}
	return a.queue.Approve(id, reviewer, comment)
}

func (a *AdminAdapter) Deny(id, reviewer, comment string) (any, error) {
	if _, err := a.Get(id); err != nil {
		return nil, err
	}
	return a.queue.Deny(id, reviewer, comment)
}

func (a *AdminAdapter) Submit(env any) (string, error) {
	e, ok := env.(*envelope.ActionEnvelope)
	if !ok || (a.scoped && (a.tenant == "" || e.Actor.TenantID != a.tenant)) {
		return "", fmt.Errorf("invalid approval scope")
	}
	return a.queue.Submit(e)
}

// ForTenant returns a view that cannot access another tenant's approvals.
func (a *AdminAdapter) ForTenant(tenant string) any {
	return &AdminAdapter{queue: a.queue, tenant: tenant, scoped: true}
}
func (a *AdminAdapter) visible(item *ApprovalItem) bool {
	return item != nil && (!a.scoped || (a.tenant != "" && item.Envelope != nil && item.Envelope.Actor.TenantID == a.tenant))
}
func (a *AdminAdapter) filter(items []*ApprovalItem) []*ApprovalItem {
	out := make([]*ApprovalItem, 0)
	for _, item := range items {
		if a.visible(item) {
			out = append(out, item)
		}
	}
	return out
}
