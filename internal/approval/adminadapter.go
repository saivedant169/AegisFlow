package approval

// AdminAdapter bridges the Queue to the admin API.
type AdminAdapter struct {
	queue *Queue
}

func NewAdminAdapter(q *Queue) *AdminAdapter {
	return &AdminAdapter{queue: q}
}

func (a *AdminAdapter) Pending() interface{} {
	return a.queue.Pending()
}

func (a *AdminAdapter) History(limit int) interface{} {
	return a.queue.History(limit)
}

func (a *AdminAdapter) Get(id string) (interface{}, error) {
	return a.queue.Get(id)
}

func (a *AdminAdapter) Approve(id, reviewer, comment string) (interface{}, error) {
	return a.queue.Approve(id, reviewer, comment)
}

func (a *AdminAdapter) Deny(id, reviewer, comment string) (interface{}, error) {
	return a.queue.Deny(id, reviewer, comment)
}
