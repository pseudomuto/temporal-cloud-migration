// Command worker runs the rehearsal's Workflow in every Namespace it is given,
// so there is live traffic, sealed payloads, and failures to watch through each
// phase of the playbook.
//
// The Rehearsal Workflow runs until it is signaled "finish". Until then it runs
// the Echo Activity every 15 seconds and on every "ping" signal, and the Fail
// Activity on every "fail" signal, so a Workflow started before a handoff keeps
// making progress after it.
//
// Environment:
//
//	TEMPORAL_ADDRESS  where to connect (default: localhost:7233, the gateway)
//	NAMESPACES        Namespaces to poll (default: "ns1 ns2 ns3")
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const taskQueue = "rehearsal"

func main() {
	addr := envOr("TEMPORAL_ADDRESS", "localhost:7233")
	namespaces := strings.Fields(envOr("NAMESPACES", "ns1 ns2 ns3"))

	var workers []worker.Worker
	for _, ns := range namespaces {
		c, err := client.Dial(client.Options{HostPort: addr, Namespace: ns})
		if err != nil {
			log.Fatalf("connecting to %s for Namespace %s: %v", addr, ns, err)
		}
		defer c.Close()

		w := worker.New(c, taskQueue, worker.Options{})
		w.RegisterWorkflowWithOptions(Rehearsal, workflow.RegisterOptions{Name: "Rehearsal"})
		w.RegisterActivity(Echo)
		w.RegisterActivity(Fail)
		if err := w.Start(); err != nil {
			log.Fatalf("starting Worker for Namespace %s: %v", ns, err)
		}
		workers = append(workers, w)
		log.Printf("polling %s on task queue %s through %s", ns, taskQueue, addr)
	}

	<-worker.InterruptCh()
	for _, w := range workers {
		w.Stop()
	}
}

// Rehearsal echoes its input back every 15 seconds and on every "ping", fails
// an Activity on every "fail", and returns everything it echoed on "finish".
func Rehearsal(ctx workflow.Context, input string) ([]string, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
	})

	var echoed []string
	echo := func(msg string) {
		var out string
		if err := workflow.ExecuteActivity(ctx, Echo, msg).Get(ctx, &out); err != nil {
			workflow.GetLogger(ctx).Error("Echo failed", "error", err)
			return
		}
		echoed = append(echoed, out)
	}

	echo(input)

	ping := workflow.GetSignalChannel(ctx, "ping")
	fail := workflow.GetSignalChannel(ctx, "fail")
	finish := workflow.GetSignalChannel(ctx, "finish")

	for ticks := 1; ; {
		done := false
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timer := workflow.NewTimer(timerCtx, 15*time.Second)

		sel := workflow.NewSelector(ctx)
		sel.AddFuture(timer, func(workflow.Future) {
			echo(fmt.Sprintf("%s (tick %d)", input, ticks))
			ticks++
		})
		sel.AddReceive(ping, func(c workflow.ReceiveChannel, _ bool) {
			var msg string
			c.Receive(ctx, &msg)
			echo(msg)
		})
		sel.AddReceive(fail, func(c workflow.ReceiveChannel, _ bool) {
			var msg string
			c.Receive(ctx, &msg)
			err := workflow.ExecuteActivity(ctx, Fail, msg).Get(ctx, nil)
			workflow.GetLogger(ctx).Info("Fail returned, as asked", "error", err)
		})
		sel.AddReceive(finish, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, nil)
			done = true
		})
		sel.Select(ctx)
		cancelTimer()

		if done {
			return echoed, nil
		}
	}
}

// Echo returns msg, so it crosses both ways as a payload.
func Echo(_ context.Context, msg string) (string, error) {
	return "echo: " + msg, nil
}

// Fail fails with msg in its message, which encryption.failures seals.
func Fail(_ context.Context, msg string) error {
	return temporal.NewNonRetryableApplicationError("rehearsal failure: "+msg, "RehearsalFailure", nil)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
