package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	dur, err := time.ParseDuration(strings.Trim(string(data), "\"'"))
	d.Duration = dur
	return err
}

type SinkSettings struct {
	Address         string
	Latency         Duration
	LossProbability float64
}

type Settings struct {
	Listen      string
	Destination SinkSettings
	Mirrors     []SinkSettings
}

func parseConfigFile(config_path string, settings *Settings) error {
	if config_path == "" {
		return nil
	}
	reader, err := os.Open(config_path)
	if err != nil {
		log.Fatalf("Can't open config '%s': %s", config_path, err)
	}
	decoder := json.NewDecoder(reader)
	err = decoder.Decode(&settings)
	if err != nil {
		log.Fatalf("Can't load settings from config '%s': %s", config_path, err)
	}
	return nil
}

func parseSinkSettings(settings string) SinkSettings {
	parts := strings.Split(settings, ";")
	if len(parts) < 1 || 3 < len(parts) {
		log.Fatalf("Can't parse sink settings '%s'", settings)
	}
	sink_settings := SinkSettings{Address: parts[0]}
	var err error
	if len(parts) >= 2 && parts[1] != "" {
		sink_settings.Latency.Duration, err = time.ParseDuration(parts[1])
		if err != nil {
			log.Fatalf("Can't parse sink latency settings '%s': %s", settings, err)
		}
	}
	if len(parts) >= 3 && parts[2] != "" {
		sink_settings.LossProbability, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			log.Fatalf("Can't parse sink loss probability settings '%s': %s", settings, err)
		}
	}
	return sink_settings
}

func exists(argument string) bool {
	short_form, long_form := "-"+argument, "--"+argument
	for _, value := range os.Args {
		if value == short_form || value == long_form {
			return true
		}
	}
	return false
}

func ParseSettings() Settings {
	var config, listen, destination string
	flag.StringVar(&config, "config", "", "Path to config file")
	flag.StringVar(&listen, "listen", "", "Address to listen on")
	flag.StringVar(&destination, "destination", "", "Address of real destination")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %[1]s:\n"+
			"%[1]s [--config CONFIG] --listen ADDRESS --destination DESTINATION "+
			"[MIRROR [MIRROR]...]\n\n"+
			"Utility resend http-request to multiply servers. Settings for destination and\n"+
			"mirrors must satistied pattern ADDRESS:LATENCY:LOSS_PROBABILITY (LATENCY and\n"+
			"LOSS_PROBABILITY are optional parameters)\n\n"+
			"positional arguments:\n  MIRROR: List of server for duplicated http-request\n\n"+
			"optional argument:\n  -h: show help usage\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	settings := Settings{}
	parseConfigFile(config, &settings)

	if exists("listen") {
		settings.Listen = listen
	}
	if exists("destination") {
		settings.Destination = parseSinkSettings(destination)
	}
	for _, mirror_settings := range flag.Args() {
		settings.Mirrors = append(settings.Mirrors, parseSinkSettings(mirror_settings))
	}
	return settings
}
