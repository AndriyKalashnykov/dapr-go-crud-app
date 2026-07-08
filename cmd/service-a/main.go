package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	fmt.Println("starting service A app")

	generateCalls(context.Background())

}

func generateCalls(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Println("generating call")
			makeRequest()
		}
	}
}

// envOr returns the value of the environment variable named key, or def when
// the variable is not set. A set-but-empty variable returns its (empty) value,
// matching the original os.LookupEnv-based defaulting.
func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// buildInvokeURL assembles a Dapr service-invocation URL from its parts. Pure
// and side-effect-free so the URL shape can be asserted in a unit test.
func buildInvokeURL(host, port, app, method string) string {
	return host + ":" + port + "/v1.0/invoke/" + app + "/method/" + method
}

func makeRequest() {
	host := envOr("DAPR_HOST", "http://localhost")
	port := envOr("DAPR_HTTP_PORT", "3500")

	order := "{\"orderId\":\"" + time.Now().String() + "\"}"

	// TODO TRY MAKE MATHOD API/HELLO
	requestURL := buildInvokeURL(host, port, "service-b", "hello")
	client := &http.Client{}
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(order))
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	// Adding app id as part of th header
	// req.Header.Add("dapr-app-id", "service-b")

	// Invoking a service
	response, err := client.Do(req)

	if err != nil {
		fmt.Print(err.Error())
		return
	}

	result, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Print(err.Error())
		return
	}

	fmt.Println("Order passed: ", string(result))
}
