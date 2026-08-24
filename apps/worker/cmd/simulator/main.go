package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"aerosight/worker/internal/simulator"
)

func main() {
	scenarioPath := flag.String("scenario", "", "path to a simulator scenario JSON file")
	realtime := flag.Bool("realtime", false, "respect step offsets in wall-clock time")
	flag.Parse()
	if *scenarioPath == "" {
		fmt.Fprintln(os.Stderr, "-scenario is required")
		os.Exit(2)
	}
	file, err := os.Open(*scenarioPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	scenario, err := simulator.DecodeScenario(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var sleeper simulator.Sleeper = simulator.NoWaitSleeper{}
	if *realtime {
		sleeper = simulator.RealTimeSleeper{}
	}
	sink := simulator.JSONSink{Encoder: json.NewEncoder(os.Stdout)}
	if err := simulator.New(sleeper).Run(context.Background(), scenario, sink); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
