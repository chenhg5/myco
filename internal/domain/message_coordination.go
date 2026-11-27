package domain

import "context"

// MessageCoordinationRepository adds metadata-only traversal and atomic queue
// transfer without expanding the repository required by older adapters.
type MessageCoordinationRepository interface {
	TaskMetadata(context.Context, string) (*Message, error)
	TaskChildren(context.Context, *Message, int) ([]*Message, error)
	ReassignTask(context.Context, string, func(*Message) (*AuditEntry, error)) (*Message, error)
}
