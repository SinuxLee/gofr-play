package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"gofr/migrations"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
	"go.mongodb.org/mongo-driver/bson"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/datasource/file/s3"
	"gofr.dev/pkg/gofr/datasource/kv-store/badger"
	"gofr.dev/pkg/gofr/datasource/mongo"
	"gofr.dev/pkg/gofr/http/response"
	"gofr.dev/pkg/gofr/service"
	"gofr.dev/pkg/gofr/websocket"
)

type Person struct {
	Name string `bson:"name" json:"name"`
	Age  int    `bson:"age" json:"age"`
	City string `bson:"city" json:"city"`
}

type Customer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type userEntity struct {
	ID   int    `json:"id,omitempty"  sql:"auto_increment"`
	Name string `json:"name"  sql:"not_null"`
	Age  int    `json:"age"`
}

func (u *userEntity) RestPath() string {
	return "users"
}

func (u *userEntity) TableName() string {
	return "user"
}

func (u *userEntity) GetAll(c *gofr.Context) (interface{}, error) {
	size := cast.ToInt(c.Param("size"))
	page := cast.ToInt(c.Param("page"))
	if size <= 0 || size > 5 {
		size = 10
	}

	if page <= 0 || page > 100 {
		page = 1
	}

	rows, err := c.SQL.QueryContext(c.Context, "select id,name,age from user limit ? offset ?", size, size*(page-1))
	if err != nil {
		return nil, err
	}

	var users []userEntity
	var user userEntity
	for rows.Next() {
		if err := rows.Scan(&user.ID, &user.Name, &user.Age); err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	return response.Raw{Data: map[string]any{"users": users, "page": page}}, nil
}

type Redirect struct{}

func (e Redirect) Error() string {
	return "Redirect"
}

func (e Redirect) StatusCode() int {
	return http.StatusMovedPermanently
}

func main() {
	db := mongo.New(mongo.Config{URI: "mongodb://localhost:27017", Database: "test", ConnectionTimeout: 4 * time.Second})
	app := gofr.New()
	app.UseMiddleware(func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inner.ServeHTTP(w, r)
		})
	})

	// app.EnableOAuth("http://jwks-endpoint", 20) // use casdoor

	app.AddHTTPService("payment",
		app.Config.GetOrDefault("PAYMENT_ADDR", "http://localhost:9000"),
		&service.CircuitBreakerConfig{
			Threshold: 4,
			Interval:  1 * time.Second,
		})

	app.Migrate(migrations.All())
	app.AddMongo(db)
	app.AddKVStore(badger.New(badger.Configs{DirPath: "badger"}))
	err := app.AddRESTHandlers(&userEntity{})
	if err != nil {
		return
	}

	app.AddFileStore(s3.New(&s3.Config{
		EndPoint:        "http://ffa-db.diandian.info:9000",
		BucketName:      "package",
		Region:          "us-east-1",
		AccessKeyID:     "minio",
		SecretAccessKey: "ffa@minio",
	}))

	// second minute hour day_of_month month day_of_week
	app.AddCronJob("*/10 * * * * *", "", func(ctx *gofr.Context) {
		ctx.Logger.Infof("current time is %v", time.Now())
	})

	app.GET("/greet", func(ctx *gofr.Context) (interface{}, error) {
		headers := map[string]string{
			"X-Custom-Header":  "CustomValue",
			"X-Another-Header": "AnotherValue",
		}

		return response.Response{
			Data:    "Hello World from new Server",
			Headers: headers,
		}, nil
	})

	app.GET("/redis", func(ctx *gofr.Context) (interface{}, error) {
		val, err := ctx.Redis.Get(ctx.Context, "greeting").Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}

		return val, nil
	})

	app.GET("/s3", func(ctx *gofr.Context) (interface{}, error) {
		dir, err := ctx.File.Getwd()
		if err == nil {
			ctx.Logf("cur dir %v", dir)
		}

		var arr []any
		entries, err := ctx.File.ReadDir("software")
		for _, entry := range entries {
			entryType := "File"

			if entry.IsDir() {
				entryType = "Dir"
			}

			ctx.Logf("%v: %v Size: %v Last Modified Time : %v", entryType, entry.Name(), entry.Size(), entry.ModTime())
			arr = append(arr, struct {
				Name  string
				Size  int64
				MTime time.Time
			}{
				entry.Name(),
				entry.Size(),
				entry.ModTime(),
			})
		}

		return arr, err
	})

	app.POST("/customer/{name}", func(ctx *gofr.Context) (interface{}, error) {
		name := ctx.PathParam("name")
		_, err := ctx.SQL.ExecContext(ctx, "INSERT INTO customers (name) VALUES (?)", name)

		return nil, err
	})

	app.GET("/customer", func(ctx *gofr.Context) (interface{}, error) {
		// Get the payment service client
		paymentSvc := ctx.GetHTTPService("payment")

		// Use the Get method to call the GET /user endpoint of payments service
		resp, err := paymentSvc.Get(ctx, "user", nil)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return string(body), nil
	})

	app.POST("/mongo", func(ctx *gofr.Context) (interface{}, error) {
		var p Person
		err := ctx.Bind(&p)
		if err != nil {
			return nil, err
		}

		res, err := ctx.Mongo.InsertOne(ctx, "person", p)
		if err != nil {
			return nil, err
		}

		return res, nil
	})
	app.GET("/mongo", func(ctx *gofr.Context) (interface{}, error) {
		var result Person

		p := ctx.Param("name")
		err := ctx.Mongo.FindOne(ctx, "person", bson.M{"name": p}, &result)
		if err != nil {
			return nil, err
		}

		return result, nil
	})

	wsUpgrader := websocket.NewWSUpgrader(
		websocket.WithHandshakeTimeout(5*time.Second), // Set handshake timeout
		websocket.WithReadBufferSize(2048),            // Set read buffer size
		websocket.WithWriteBufferSize(2048),           // Set write buffer size
		websocket.WithSubprotocols("chat", "binary"),  // 约定 二进制传输的 chat 格式
		websocket.WithCompression(),                   // Enable compression
	)

	app.OverrideWebsocketUpgrader(wsUpgrader)
	app.WebSocket("/ws", WSHandler)
	app.GET("/", func(ctx *gofr.Context) (interface{}, error) {
		return nil, &Redirect{}
	})

	app.Run() // http://localhost:8086/static/
}

func WSHandler(ctx *gofr.Context) (interface{}, error) {
	con := ctx.Context.Value(websocket.WSConnectionKey).(*websocket.Connection)
	user := &userEntity{}
	if err := ctx.Bind(user); err != nil {
		ctx.Logger.Errorf("Error binding message: %v", err)
		return nil, err
	}

	ctx.Logger.Infof("Received message: %s", user.Name)

	data, _ := json.Marshal(user)
	con.WriteMessage(2, data)
	return []byte{}, nil
}
