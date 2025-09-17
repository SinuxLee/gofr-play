package migrations

import (
	"context"

	"gofr.dev/pkg/gofr/migration"
)

func initRedisLuaScript() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.Redis.Set(context.Background(), "gift_counter", 100000, 0).Result()
			if err != nil {
				return err
			}
			return nil
		},
	}
}
