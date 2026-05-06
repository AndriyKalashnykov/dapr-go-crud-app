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

// selectStorage maps the --connStr flag to a concrete storage backend.
// Pure (modulo the panicking Mongo client constructor) so the routing
// table can be table-tested for the in-memory and Dapr branches.
func selectStorage(connStr string, maxItems int) storage.TodosStorage {
	switch connStr {
	case "", "mem":
		fmt.Println("Using in-memory storage")
		return storage.NewInMemoryStorage(maxItems)
	case "dapr":
		fmt.Println("Using Dapr state store")
		return storage.NewDaprStorage(maxItems)
	default:
		fmt.Printf("Using MongoDB storage: %s\n", connStr)
		return storage.NewMongoStorage(connStr, maxItems)
	}
}

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
		Exec: func(_ context.Context, _ []string) error {
			s := selectStorage(*connStr, *maxItems)
			(&server.Server{
				Port:                   *serverPort,
				Storage:                s,
				CleanupIntervalSeconds: *cleanupIntervalSeconds,
			}).Start()
			return nil
		},
	}

	root := &ffcli.Command{
		Name:        "app",
		LongHelp:    "management cli for basic crud-app",
		Subcommands: []*ffcli.Command{serve},
		// Without an Exec on the root command, ffcli panics when invoked
		// with no subcommand ("terminal command doesn't define an Exec
		// function"). A clean exit prints usage and returns — supports
		// `docker run app` smoke tests + `app --help`.
		Exec: func(_ context.Context, _ []string) error {
			fmt.Fprintf(os.Stderr, "usage: app serve [-port N] [-connStr STRING] [-maxItems N] [-cleanupIntervalSeconds N]\n")
			return nil
		},
	}

	if err := root.ParseAndRun(context.Background(), os.Args[1:]); err != nil {
		panic(err)
	}
}
