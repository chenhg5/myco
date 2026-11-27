package sqlite

import "database/sql"

type schemaConnection interface {
	Query(string, ...any) (*sql.Rows, error)
	Exec(string, ...any) (sql.Result, error)
}

func migrateMessageCoordination(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existed int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='ever_delivered'`).Scan(&existed); err != nil {
		return err
	}
	if err := addColumns(tx, "messages", [][2]string{{"parent_id", "TEXT NOT NULL DEFAULT ''"}, {"reassign_note", "TEXT NOT NULL DEFAULT ''"}, {"ever_delivered", "INTEGER NOT NULL DEFAULT 0"}}); err != nil {
		return err
	}
	if existed == 0 {
		// A pre-migration queued task may already have been disclosed by a push
		// or inbox wait without any claim/ACK or timestamp change. Only tasks
		// created after this guard exists can prove they have not been delivered.
		// Terminal rows cannot be reassigned and need no legacy rewrite.
		if _, err := tx.Exec(`UPDATE messages SET ever_delivered=1 WHERE type='task' AND status NOT IN ('done','failed')`); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_parent_task ON messages(parent_id, created_at, id) WHERE type='task' AND parent_id<>''`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
