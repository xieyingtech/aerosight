package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aerosight/worker/internal/dji"
	"aerosight/worker/internal/simulator"
)

func main() {
	mode := flag.String("mode", "stdout", "simulator mode: stdout or dji-mqtt")
	scenarioPath := flag.String("scenario", "", "path to a simulator scenario JSON file")
	realtime := flag.Bool("realtime", false, "respect step offsets in wall-clock time")
	topologyPath := flag.String("topology", "", "DJI topology fixture path (dji-mqtt mode)")
	product := flag.String("product", "", "built-in DJI product scenario, for example dock2-m3td")
	gatewaySN := flag.String("gateway-sn", "", "DJI gateway serial number (dji-mqtt mode)")
	aircraftSN := flag.String("aircraft-sn", "", "DJI aircraft serial number for a built-in scenario")
	mqttURL := flag.String("mqtt-url", "mqtt://127.0.0.1:1883", "MQTT broker URL without inline credentials")
	mqttUsername := flag.String("mqtt-username", "", "MQTT username")
	mqttPasswordEnv := flag.String("mqtt-password-env", "AEROSIGHT_DJI_SIM_MQTT_PASSWORD", "environment variable containing the MQTT password")
	nackMethod := flag.String("nack-method", "", "DJI service method that should return a NACK")
	timeoutMethod := flag.String("timeout-method", "", "DJI service method whose reply should be dropped")
	disconnectAfter := flag.Duration("disconnect-after", 0, "exit after this duration to simulate a device disconnect")
	unknownFirmware := flag.String("unknown-firmware", "", "replace product firmware with an unvalidated version")
	unknownCapability := flag.String("unknown-capability", "", "inject an unknown runtime capability in aircraft telemetry")
	ffmpegExecutable := flag.String("ffmpeg-executable", "", "FFmpeg executable or wrapper used for DJI live push")
	mediaHostOverride := flag.String("media-host-override", "", "replace the RTMP hostname passed to the media process")
	flag.Parse()
	if *mode == "dji-mqtt" {
		runDJIMQTT(*topologyPath, *product, *gatewaySN, *aircraftSN, *mqttURL, *mqttUsername, *mqttPasswordEnv,
			*nackMethod, *timeoutMethod, *disconnectAfter, *unknownFirmware, *unknownCapability, *ffmpegExecutable, *mediaHostOverride)
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

func runDJIMQTT(topologyPath, product, gatewaySN, aircraftSN, brokerURL, username, passwordEnv, nackMethod, timeoutMethod string,
	disconnectAfter time.Duration, unknownFirmware, unknownCapability string,
	ffmpegExecutable, mediaHostOverride string,
) {
	if (topologyPath == "" && product == "") || gatewaySN == "" || username == "" || passwordEnv == "" {
		fmt.Fprintln(os.Stderr, "-gateway-sn, -mqtt-username, -mqtt-password-env, and either -topology or -product are required in dji-mqtt mode")
		os.Exit(2)
	}
	password := os.Getenv(passwordEnv)
	if password == "" {
		fmt.Fprintf(os.Stderr, "MQTT password environment variable %s is empty\n", passwordEnv)
		os.Exit(2)
	}
	protocolConfig := simulator.DJIProtocolConfig{GatewaySN: gatewaySN, AircraftSN: aircraftSN}
	if product != "" {
		var err error
		if strings.HasPrefix(product, "dock3-") {
			protocolConfig, err = simulator.Dock3Scenario(product, gatewaySN, aircraftSN, time.Now())
		} else {
			protocolConfig, err = simulator.Dock2Scenario(product, gatewaySN, aircraftSN, time.Now())
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if unknownFirmware != "" {
			protocolConfig, err = simulator.InjectUnknownFirmware(protocolConfig, unknownFirmware)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	} else {
		topology, err := os.ReadFile(topologyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		protocolConfig.Topology = topology
	}
	protocolConfig.Faults = simulator.DJIFaults{
		NackMethods: map[string]int{nackMethod: 1}, TimeoutMethods: map[string]bool{timeoutMethod: true},
		UnknownCapability: unknownCapability,
	}
	delete(protocolConfig.Faults.NackMethods, "")
	delete(protocolConfig.Faults.TimeoutMethods, "")
	if ffmpegExecutable != "" {
		media, err := simulator.NewFFmpegMediaController(ffmpegExecutable, mediaHostOverride)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer media.Close()
		protocolConfig.Media = media
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
	protocol, err := simulator.NewDJIProtocol(protocolConfig, session)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var telemetryErrors <-chan error
	var disconnect <-chan time.Time
	if disconnectAfter > 0 {
		disconnect = time.After(disconnectAfter)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-session.Done():
			fmt.Fprintln(os.Stderr, "DJI MQTT simulator session ended")
			return
		case <-disconnect:
			fmt.Fprintln(os.Stderr, "DJI simulator: injected disconnect")
			return
		case err := <-telemetryErrors:
			if err != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, err)
			}
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
				if protocolConfig.TelemetryInterval > 0 && telemetryErrors == nil {
					channel := make(chan error, 1)
					telemetryErrors = channel
					go func() { channel <- protocol.RunTelemetry(ctx) }()
				}
			}
		}
	}
}
