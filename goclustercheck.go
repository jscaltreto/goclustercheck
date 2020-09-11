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
var username = flag.String("username", "clustercheckuser", "MySQL Username")
var password = flag.String("password", "clustercheckpassword!", "MySQL Password")
var socket = flag.String("socket", "", "MySQL UNIX Socket")
var host = flag.String("host", "", "MySQL Server")
var port = flag.Int("port", 3306, "MySQL Port")
var timeout = flag.Duration("timeout", defaultTimeout, "MySQL connection timeout")
var checkInterval = flag.Duration("checkInterval", defaultCheckInterval, "How often to check mysql status")
var availableWhenDonor = flag.Bool("donor", false, "Cluster available while node is a donor")
var availableWhenReadonly = flag.Bool("readonly", false, "Cluster available while node is read only")
var forceFailFile = flag.String("failfile", "/dev/shm/proxyoff", "Create this file to manually fail checks")
var forceUpFile = flag.String("upfile", "/dev/shm/proxyon", "Create this file to manually pass checks")
var bindPort = flag.Int("bindport", 9200, "MySQLChk bind port")
var bindAddr = flag.String("bindaddr", "", "MySQLChk bind address")

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
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, *mysqlBin, *commandArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("Failed to get status: %q", stderr.String())
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

func main() {
	flag.Parse()

	checkCommandArgs = []string{
		"-n", "-N", "-s",
		fmt.Sprintf("--user=%s", *username),
		fmt.Sprintf("--password=%s", *password),
		fmt.Sprintf("--connect-timeout=%d", int(timeout.Seconds())),
	}
	if *socket != "" {
		checkCommandArgs = append(checkCommandArgs, fmt.Sprintf("--socket=%s", *socket))
	} else if *host != "" {
		checkCommandArgs = append(checkCommandArgs, fmt.Sprintf("--host=%s", *host))
		checkCommandArgs = append(checkCommandArgs, fmt.Sprintf("--port=%d", *port))
	}

	query := "show status where Variable_name in ('read_only', 'wsrep_local_state', 'wsrep_local_state_comment');"
	checkCommandArgs = append(checkCommandArgs, "-e", query)

	// Get Initial State
	currentState = checkWsrep()

	go func() {
		for range time.Tick(*checkInterval) {
			lastState := currentState
			currentState = checkWsrep()
			if lastState.comment != currentState.comment {
				log.Printf("Status changed! Now \"%s\", Was \"%s\"\n", currentState.comment, lastState.comment)
			}
		}
	}()

	http.HandleFunc("/", checkHandler)

	bind := fmt.Sprintf("%s:%d", *bindAddr, *bindPort)
	log.Printf("Listening on %s\n", bind)
	log.Fatal(http.ListenAndServe(bind, nil))
}
