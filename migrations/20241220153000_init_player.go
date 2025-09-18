package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const (
	createPlayerTableSQL = `CREATE TABLE IF NOT EXISTS t_player (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    create_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	update_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_player
AFTER UPDATE ON t_player
FOR EACH ROW
BEGIN
    UPDATE t_player SET update_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;

INSERT INTO t_player(name) VALUES ('initial_record');
UPDATE sqlite_sequence SET seq = 99999 WHERE name = 't_player';
DELETE FROM t_player WHERE name = 'initial_record';
`
)

func createPlayerTable() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createPlayerTableSQL)
			return err
		},
	}
}
