package runner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/myco-ai/myco/internal/domain"
)

func (r *Runner) heldWakeContext(ctx context.Context, log *slog.Logger, id string) (string, []string) {
	if id == domain.RecoveredHold {
		return "\n\n## Task resumed\nThis task was automatically resumed because it was held without a wake condition. Check what was already done and continue. Wait in the foreground for commands or CI, use ask for a decision, or use hold --until for a delegated task.\n", nil
	}
	m, err := r.hub.GetMessage(ctx, id)
	if err != nil || m.Unreadable != "" {
		return fmt.Sprintf("\n\n## Task resumed\nIts wake message is %s. Read it with myco message get before continuing.\n", id), nil
	}
	if m.Type == "task" {
		return fmt.Sprintf("\n\n## Task resumed\nDelegated task %s ended with status %s without a reply. Use this outcome to continue the original task.\n", m.ID, m.Status), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## Task resumed\nReceived %s %s from %s. Continue the original task with this information.\n\n%s\n", m.Type, m.ID, m.From, m.Body)
	files, dirs := r.downloadAttachments(ctx, log, *m)
	for _, file := range files {
		if file.Error != "" {
			fmt.Fprintf(&b, "- Attachment %s: download failed (%s); use myco file get --id %s\n", file.Attachment.Name, file.Error, file.Attachment.FileID)
		} else {
			fmt.Fprintf(&b, "- Attachment %s: %s\n", file.Attachment.Name, file.Path)
		}
	}
	return b.String(), dirs
}
