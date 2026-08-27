package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"aerosight/worker/internal/dji"
	"aerosight/worker/internal/simulator"
)

func main() {
	mode := flag.String("mode", "stdout", "simulator mode: stdout or dji-mqtt")
	scenarioPath := flag.String("scenario", "", "path to a simulator scenario JSON file")
	realtime := flag.Bool("realtime", false, "respect step offsets in wall-clock time")
	topologyPath := flag.String("topology", "", "DJI topology fixture path (dji-mqtt mode)")
	gatewaySN := flag.String("gateway-sn", "", "DJI gateway serial number (dji-mqtt mode)")
	mqttURL := flag.String("mqtt-url", "mqtt://127.0.0.1:1883", "MQTT broker URL without inline credentials")
	mqttUsername := flag.String("mqtt-username", "", "MQTT username")
	mqttPasswordEnv := flag.String("mqtt-password-env", "AEROSIGHT_DJI_SIM_MQTT_PASSWORD", "environment variable containing the MQTT password")
	flag.Parse()
	if *mode == "dji-mqtt" {
		runDJIMQTT(*topologyPath, *gatewaySN, *mqttURL, *mqttUsername, *mqttPasswordEnv)
		return
	}
	if *mode != "stdout" {
		fmt.Fprintf(os.Stderr, "unsupported -mode %q\n", *mode)
		os.Exit(2)
	}
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

func runDJIMQTT(topologyPath, gatewaySN, brokerURL, username, passwordEnv string) {
	if topologyPath == "" || gatewaySN == "" || username == "" || passwordEnv == "" {
		fmt.Fprintln(os.Stderr, "-topology, -gateway-sn, -mqtt-username, and -mqtt-password-env are required in dji-mqtt mode")
		os.Exit(2)
	}
	password := os.Getenv(passwordEnv)
	if password == "" {
		fmt.Fprintf(os.Stderr, "MQTT password environment variable %s is empty\n", passwordEnv)
		os.Exit(2)
	}
	topology, err := os.ReadFile(topologyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	incoming := make(chan dji.MQTTMessage, 32)
	session, err := dji.StartMQTTSession(ctx, dji.MQTTConfig{
		BrokerURL: brokerURL, ClientID: "aerosight-dji-simulator-" + gatewaySN,
		Username: username, Password: []byte(password), Topics: []string{"thing/product/" + gatewaySN + "/services"},
	}, func(messageCtx context.Context, message dji.MQTTMessage) error {
		select {
		case incoming <- message:
			return nil
		case <-messageCtx.Done():
			return messageCtx.Err()
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	protocol, err := simulator.NewDJIProtocol(simulator.DJIProtocolConfig{GatewaySN: gatewaySN, Topology: topology}, session)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-session.Done():
			fmt.Fprintln(os.Stderr, "DJI MQTT simulator session ended")
			return
		case message := <-incoming:
			if err := protocol.HandleMessage(ctx, message.Topic, message.Payload); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		case event := <-session.Events():
			fmt.Fprintf(os.Stderr, "DJI simulator: %s (%s)\n", event.State, event.Code)
			if event.State == "connected" {
				if err := protocol.PublishTopology(ctx); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}
		}
	}
}
