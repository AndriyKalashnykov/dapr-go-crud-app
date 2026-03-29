package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/server"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
	"github.com/peterbourgon/ff/v3/ffcli"
)

func main() {

	serveFlagSet := flag.NewFlagSet("app serve", flag.ExitOnError)
	serverPort := serveFlagSet.Int("port", 8080, "port for the server to listen to")
	connStr := serveFlagSet.String("connStr", "mongodb://localhost:27017", "connection string for storage")
	maxItems := serveFlagSet.Int("maxItems", 20, "maximum numbers of items to store")
	cleanupIntervalSeconds := serveFlagSet.Int("cleanupIntervalSeconds", 600, "seconds to wait between cleanup runs")

	serve := &ffcli.Command{
		Name:     "serve",
		LongHelp: "Runs the http server for this app",
		FlagSet:  serveFlagSet,
		Exec: func(ctx context.Context, args []string) error {

			var s storage.TodosStorage

			switch *connStr {
			case "", "mem":
				fmt.Println("Using inmemory storage")
				s = storage.NewInMemoryStorage(*maxItems)
			case "dapr":
				s = storage.NewDaprStorage(*maxItems)
			default:
				s = storage.NewMongoStorage(*connStr, *maxItems)
			}

			server := server.Server{
				Port:                   *serverPort,
				Storage:                s,
				CleanupIntervalSeconds: *cleanupIntervalSeconds,
			}
			server.Start()
			return nil
		},
	}

	root := &ffcli.Command{
		Name:        "app",
		LongHelp:    "management cli for basic crud-app",
		Subcommands: []*ffcli.Command{serve},
	}

	if err := root.ParseAndRun(context.Background(), os.Args[1:]); err != nil {
		panic(err)
	}

}
