//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"

	tc "github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// This is the "middle layer" integration test for DaprStorage: it exercises
// the real Dapr state-store + pub/sub code path (pkg/storage/dapr.go) against
// a REAL Redis fronted by a REAL daprd sidecar — the gap between the
// fake-client unit tests (dapr_test.go) and the slow full-KinD e2e.
//
// Wiring: a private docker network hosts a Redis container (alias `redis`) and
// a daprd container whose --resources-path points at a components dir declaring
// a `state.redis` (name `statestore`) + `pubsub.redis` (name `pubsub`) both
// pointed at `redis:6379`. daprd's gRPC port is published to the host; the Dapr
// Go SDK (`dapr.NewClient()`, invoked lazily by DaprStorage.newDaprClient) dials
// it via the DAPR_GRPC_PORT env var. Component names/topic match the constants
// in dapr.go (statestore / pubsub / todos).
//
// NOTE — the Dapr Go SDK's `dapr.NewClient()` is a process-wide sync.Once
// singleton keyed on the DAPR_GRPC_PORT read at first call. So the whole binary
// gets ONE daprd + ONE port, set once here; scenarios run as subtests that
// FLUSHALL Redis for isolation rather than each spinning a fresh sidecar.

// env reads an operator-tunable value with a documented default (mirrors the
// .env-fallback idiom in rules/common/configuration.md). The defaults keep the
// test hermetic with zero external config.
func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// daprInfra is one Redis + one daprd wired on a shared network, plus a handle
// to run redis-cli inside the Redis container for direct broker assertions.
type daprInfra struct {
	redis        tc.Container
	daprGRPCPort string // host-mapped daprd gRPC port
}

// dockerUnavailable reports whether err looks like "Docker isn't usable here"
// (daemon down / socket missing / binary absent) so the suite can t.Skip
// cleanly instead of failing on infrastructure the runner doesn't provide.
func dockerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, marker := range []string{
		"Cannot connect to the Docker daemon",
		"cannot connect to the Docker daemon",
		"docker daemon is not running",
		"failed to find a Docker",
		"no such file or directory",
		"error during connect",
		"permission denied while trying to connect",
		"Is the docker daemon running",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// componentFiles writes the state.redis + pubsub.redis component YAMLs (pointed
// at redisAddr) into a fresh temp dir and returns per-file ContainerFile copy
// specs targeting /components/<name>.yaml inside daprd. Copying files
// individually (vs. a whole dir) makes testcontainers create the /components
// parent for us. Shapes mirror .dapr/components/{state,pubsub}.yaml
// (type/version) minus the k8s-only secretKeyRef/scopes — a single-app test
// needs no scoping.
func componentFiles(t *testing.T, redisAddr string) []tc.ContainerFile {
	t.Helper()
	dir := t.TempDir()

	state := fmt.Sprintf(`apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: statestore
spec:
  type: state.redis
  version: v1
  metadata:
  - name: redisHost
    value: %q
  - name: redisPassword
    value: ""
`, redisAddr)

	pubsub := fmt.Sprintf(`apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: pubsub
spec:
  type: pubsub.redis
  version: v1
  metadata:
  - name: redisHost
    value: %q
  - name: redisPassword
    value: ""
`, redisAddr)

	statePath := filepath.Join(dir, "statestore.yaml")
	pubsubPath := filepath.Join(dir, "pubsub.yaml")
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatalf("write statestore component: %v", err)
	}
	if err := os.WriteFile(pubsubPath, []byte(pubsub), 0o644); err != nil {
		t.Fatalf("write pubsub component: %v", err)
	}
	return []tc.ContainerFile{
		{HostFilePath: statePath, ContainerFilePath: "/components/statestore.yaml", FileMode: 0o644},
		{HostFilePath: pubsubPath, ContainerFilePath: "/components/pubsub.yaml", FileMode: 0o644},
	}
}

// startDaprInfra brings up the network + Redis + daprd, registers Terminate
// cleanups, and returns the wired handle. On a docker-unavailable error it
// t.Skips; any other startup error is a hard t.Fatal.
func startDaprInfra(t *testing.T) *daprInfra {
	t.Helper()

	redisImage := env("REDIS_IMAGE", "redis:8-alpine")
	daprdImage := env("DAPRD_IMAGE", "daprio/daprd:1.17.1")
	// Internal container ports (daprd gRPC/HTTP inside the network) — the
	// host-mapped ports are ephemeral/dynamic by design.
	daprGRPCInternal := env("DAPRD_GRPC_PORT", "50001")
	daprHTTPInternal := env("DAPRD_HTTP_PORT", "3500")
	redisInternal := env("REDIS_PORT", "6379")

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	nw, err := tcnetwork.New(ctx)
	if err != nil {
		if dockerUnavailable(err) {
			t.Skipf("Docker unavailable, skipping Dapr integration test: %v", err)
		}
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = nw.Remove(ctx)
	})

	// --- Redis ---
	redisC, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:          redisImage,
			ExposedPorts:   []string{redisInternal + "/tcp"},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"redis"}},
			WaitingFor:     wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
		},
	})
	if err != nil {
		if dockerUnavailable(err) {
			t.Skipf("Docker unavailable, skipping Dapr integration test: %v", err)
		}
		t.Fatalf("start redis container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := redisC.Terminate(ctx); err != nil {
			t.Logf("terminate redis container: %v", err)
		}
	})

	// --- daprd sidecar ---
	compFiles := componentFiles(t, "redis:"+redisInternal)
	daprdC, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:        daprdImage,
			ExposedPorts: []string{daprGRPCInternal + "/tcp"},
			Networks:     []string{nw.Name},
			Entrypoint:   []string{"/daprd"},
			Cmd: []string{
				"--app-id", "test",
				"--dapr-grpc-port", daprGRPCInternal,
				"--dapr-http-port", daprHTTPInternal,
				"--resources-path", "/components",
				"--log-level", "info",
			},
			Files: compFiles,
			WaitingFor: wait.ForAll(
				wait.ForLog("dapr initialized. Status: Running").WithStartupTimeout(90*time.Second),
				wait.ForListeningPort(daprGRPCInternal+"/tcp").WithStartupTimeout(90*time.Second),
			),
		},
	})
	if err != nil {
		if dockerUnavailable(err) {
			t.Skipf("Docker unavailable, skipping Dapr integration test: %v", err)
		}
		t.Fatalf("start daprd container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := daprdC.Terminate(ctx); err != nil {
			t.Logf("terminate daprd container: %v", err)
		}
	})

	mapped, err := daprdC.MappedPort(ctx, daprGRPCInternal+"/tcp")
	if err != nil {
		t.Fatalf("mapped daprd gRPC port: %v", err)
	}

	return &daprInfra{redis: redisC, daprGRPCPort: mapped.Port()}
}

// flushRedis clears all keys so each subtest starts from an empty state store
// (the DaprStorage `index` key + every todo key live in this single Redis).
func (d *daprInfra) flushRedis(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	code, _, err := d.redis.Exec(ctx, []string{"redis-cli", "FLUSHALL"}, tcexec.Multiplexed())
	if err != nil || code != 0 {
		t.Fatalf("FLUSHALL: code=%d err=%v", code, err)
	}
}

// redisXLen returns XLEN of a stream key inside Redis (dapr's redis pub/sub
// XADDs each published event to a stream named after the topic). Missing key
// XLEN is 0.
func (d *daprInfra) redisXLen(t *testing.T, stream string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	code, r, err := d.redis.Exec(ctx, []string{"redis-cli", "XLEN", stream}, tcexec.Multiplexed())
	if err != nil || code != 0 {
		t.Fatalf("XLEN %s: code=%d err=%v", stream, code, err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read XLEN output: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(buf.String()))
	if err != nil {
		t.Fatalf("parse XLEN output %q: %v", buf.String(), err)
	}
	return n
}

func TestDaprStorage_Integration(t *testing.T) {
	infra := startDaprInfra(t)

	// Point the Dapr Go SDK at the host-mapped daprd gRPC port. Set ONCE,
	// before any DaprStorage method triggers the sync.Once dapr.NewClient().
	t.Setenv("DAPR_GRPC_PORT", infra.daprGRPCPort)

	t.Run("CreateThenListAll_RoundTrip", func(t *testing.T) {
		infra.flushRedis(t)
		s := storage.NewDaprStorage(10)

		td := &todos.Todo{Text: "buy milk"}
		if err := s.Create(td); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if td.Id == "" {
			t.Fatal("Create did not assign Id")
		}

		all, err := s.ListAll()
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		if len(all) != 1 || all[0].Id != td.Id || all[0].Text != "buy milk" {
			t.Fatalf("ListAll = %+v, want single round-tripped todo (id=%s)", all, td.Id)
		}
	})

	t.Run("Create_EvictsOldestAtMaxItems", func(t *testing.T) {
		infra.flushRedis(t)
		const max = 3
		s := storage.NewDaprStorage(max)

		texts := []string{"a", "b", "c", "d", "e"}
		for _, text := range texts {
			if err := s.Create(&todos.Todo{Text: text}); err != nil {
				t.Fatalf("Create(%s): %v", text, err)
			}
		}

		all, err := s.ListAll()
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		if len(all) != max {
			t.Fatalf("ListAll len = %d, want %d (FIFO cap)", len(all), max)
		}
		// FIFO: the two oldest ("a","b") evicted; newest 3 retained in order.
		wantTexts := texts[2:]
		for i, td := range all {
			if td.Text != wantTexts[i] {
				t.Errorf("ListAll[%d].Text = %q, want %q (insertion order after eviction)", i, td.Text, wantTexts[i])
			}
		}
	})

	t.Run("Delete_RemovesTodo", func(t *testing.T) {
		infra.flushRedis(t)
		s := storage.NewDaprStorage(10)

		keep := &todos.Todo{Text: "keep"}
		drop := &todos.Todo{Text: "drop"}
		if err := s.Create(keep); err != nil {
			t.Fatalf("Create(keep): %v", err)
		}
		if err := s.Create(drop); err != nil {
			t.Fatalf("Create(drop): %v", err)
		}

		if err := s.Delete(drop); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		all, err := s.ListAll()
		if err != nil {
			t.Fatalf("ListAll after Delete: %v", err)
		}
		if len(all) != 1 || all[0].Id != keep.Id {
			t.Fatalf("ListAll after Delete = %+v, want only kept todo (id=%s)", all, keep.Id)
		}
		for _, td := range all {
			if td.Id == drop.Id {
				t.Errorf("deleted todo %s still present after Delete", drop.Id)
			}
		}
	})

	t.Run("Create_And_Delete_PublishToRealBroker", func(t *testing.T) {
		infra.flushRedis(t)
		s := storage.NewDaprStorage(10)

		td := &todos.Todo{Text: "event-emitting"}
		if err := s.Create(td); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Delete(td); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// publishEvent (Create + Delete) went through real daprd → real Redis
		// pub/sub; each XADDs to the `todos` topic stream. Two publishes ⇒
		// XLEN >= 2. Assert the broker actually received them (the side-effect
		// the fake-client unit test can only count in memory).
		if got := infra.redisXLen(t, "todos"); got < 2 {
			t.Fatalf("XLEN todos = %d, want >= 2 (Create + Delete published to real broker)", got)
		}
	})
}
