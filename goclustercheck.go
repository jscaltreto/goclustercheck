// goclustercheck is a small service that attempts to replicate the behavior
// of "percona-clustercheck" (https://github.com/olafz/percona-clustercheck).
// That is, it will periodically check the status of a Percona XtraDB Cluster
// node and make that status available in a pass/fail way over HTTP.

// The easiest way to run this is under systemd. README.md contains a sample
// unit file.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type statusValues map[string]string

type state struct {
	available bool
	comment   string
}

var currentState state
var checkCommandArgs []string

var defaultTimeout = 10 * time.Second
var defaultCheckInterval = 5 * time.Second

var mysqlBin = flag.String("mysql-binary", "mysql", "Path to mysql binary")
var timeout = flag.Duration("timeout", defaultTimeout, "Status check timeout")
var checkInterval = flag.Duration("checkInterval", defaultCheckInterval, "How often to check mysql status")
var availableWhenDonor = flag.Bool("donor", false, "Available while node is a donor")
var availableWhenReadonly = flag.Bool("readonly", false, "Available while node is read only")
var forceFailFile = flag.String("failfile", "/dev/shm/proxyoff", "Force fail")
var forceUpFile = flag.String("upfile", "/dev/shm/proxyon", "Force pass")
var bindPort = flag.Int("bindport", 9200, "HTTP bind port")
var bindAddr = flag.String("bindaddr", "", "HTTP bind address")

func checkHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(*forceUpFile); err == nil {
		fmt.Fprint(w, "Cluster node OK by manual override")
		return
	}

	if _, err := os.Stat(*forceFailFile); err == nil {
		http.Error(w, "Cluster node unavailable by manual override", http.StatusServiceUnavailable)
		return
	}

	if currentState.available {
		fmt.Fprint(w, currentState.comment)
		return
	}
	http.Error(w, currentState.comment, http.StatusServiceUnavailable)
}

func getStatus(commandArgs *[]string) (statusValues, error) {
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, *mysqlBin, *commandArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("Timed out waiting for status query to complete")
		}
		return nil, fmt.Errorf("Failed to get status: %s (stderr: %q)", err, stderr.String())
	}

	propVals := statusValues{}
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		statusVar := strings.Fields(scanner.Text())
		propVals[statusVar[0]] = statusVar[1]
	}
	return propVals, nil
}

func checkWsrep() state {
	wsrepState, err := getStatus(&checkCommandArgs)
	if err != nil {
		return state{false, err.Error()}
	}
	localState, ok := wsrepState["wsrep_local_state"]
	if !ok {
		return state{false, "Unable to determine wsrep state"}
	}

	available := localState == "4"
	if *availableWhenDonor {
		available = available || localState == "2"
	}
	if !*availableWhenReadonly {
		readOnly, ok := wsrepState["read_only"]
		if ok && readOnly == "ON" {
			return state{false, "Read Only"}
		}
	}

	return state{available, wsrepState["wsrep_local_state_comment"]}
}

func updateState() {
	lastState := currentState
	currentState = checkWsrep()
	if lastState.comment != currentState.comment {
		log.Printf("Status changed! Now \"%s\", Was \"%s\"\n", currentState.comment, lastState.comment)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Println("\nAny agruments following \"--\" are passed directly to the mysql command")
		fmt.Println("and can be used to specify command-line options such has host, port, user, etc.")
	}

	flag.Parse()

	checkCommandArgs = os.Args[len(os.Args)-flag.NArg():]
	checkCommandArgs = append(checkCommandArgs, "-n", "-N", "-s")

	query := "show status where Variable_name in ('read_only', 'wsrep_local_state', 'wsrep_local_state_comment');"
	checkCommandArgs = append(checkCommandArgs, "-e", query)

	currentState = state{false, "Initializing"}

	go func() {
		// Get Initial State
		updateState()
		for range time.Tick(*checkInterval) {
			updateState()
		}
	}()

	http.HandleFunc("/", checkHandler)

	bind := fmt.Sprintf("%s:%d", *bindAddr, *bindPort)
	log.Printf("Listening on %s\n", bind)
	log.Fatal(http.ListenAndServe(bind, nil))
}
