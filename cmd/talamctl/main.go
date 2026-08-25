// Command talamctl is a minimal CLI against talam-server's Fleet API — the
// scriptable counterpart to the dashboard (docs/components/server/README.md).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	server := flag.String("server", envOr("TALAM_SERVER", "http://localhost:8443"), "talam-server base URL")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "incidents":
		get(*server, "/v1/incidents")
	case "proposals":
		q := ""
		if len(args) > 1 {
			q = "?state=" + args[1]
		}
		get(*server, "/v1/proposals"+q)
	case "stats":
		get(*server, "/v1/stats")
	case "approve", "reject":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: talamctl %s <proposal-id> <decided-by> [reason]\n", args[0])
			os.Exit(2)
		}
		reason := ""
		if len(args) > 3 {
			reason = args[3]
		}
		decide(*server, args[1], args[0] == "approve", args[2], reason)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `talamctl [-server URL] <command>

Commands:
  incidents                          list incidents, newest first
  proposals [state]                  list remediation proposals, optionally filtered (Pending|Approved|Rejected|Applied|Failed)
  stats                              fleet summary
  approve <id> <decided-by> [reason] approve a pending proposal
  reject  <id> <decided-by> [reason] reject a pending proposal`)
}

func get(server, path string) {
	resp, err := http.Get(server + path)
	fatalIf(err)
	defer resp.Body.Close()
	printPretty(resp)
}

func decide(server, id string, approve bool, decidedBy, reason string) {
	body, _ := json.Marshal(map[string]any{"approve": approve, "decidedBy": decidedBy, "reason": reason})
	resp, err := http.Post(server+"/v1/proposals/"+id+"/decision", "application/json", bytes.NewReader(body))
	fatalIf(err)
	defer resp.Body.Close()
	printPretty(resp)
}

func printPretty(resp *http.Response) {
	raw, err := io.ReadAll(resp.Body)
	fatalIf(err)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "%s: %s\n", resp.Status, raw)
		os.Exit(1)
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(pretty))
		return
	}
	fmt.Println(string(raw))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
