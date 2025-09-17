package migrations

import "gofr.dev/pkg/gofr/migration"

const createTable = `CREATE TABLE IF NOT EXISTS user (
	id BIGINT PRIMARY KEY AUTO_INCREMENT,
	name VARCHAR(200) not null ,
	age INT not null
);`

func createTableEmployee() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createTable)
			if err != nil {
				return err
			}
			return nil
		},
	}
}
