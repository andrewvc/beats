package rserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/elastic/beats/v7/heartbeat/eventext"
	"github.com/elastic/beats/v7/heartbeat/monitors/jobs"
	"github.com/elastic/beats/v7/heartbeat/monitors/plugin"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
	"net/http"
	"time"
)

func init() {
	plugin.Register("rclient", create)
}

type rclientConfig struct {
	Locations map[string]string `config:"locations"`
	Config common.MapStr `config:"config"`
}

func create(name string, cfg *common.Config) (p plugin.Plugin, err error) {
	config := &rclientConfig{}
	cfg.Unpack(config)

	return plugin.Plugin{
		Endpoints: 0,
		Close: nil,
		Jobs: runConfig(config),
	}, nil
}

func runConfig(config *rclientConfig) (js []jobs.Job) {
	for loc, locUrl := range config.Locations {
		loc := loc
		js = append(js, func(event *beat.Event) ([]jobs.Job, error) {
			body, err := json.Marshal(config.Config)
			if err != nil {
				return nil, fmt.Errorf("could not marshal rclient body: %w", err)
			}
			resp, err := http.Post(locUrl, "text/json", bytes.NewReader(body))
			logp.Warn("POST DONE %s", loc)
			if err != nil {
				return nil, fmt.Errorf("error on post: %w", err)
			}

			scanner := bufio.NewScanner(resp.Body)
			var scanJobs []jobs.Job
			if scanner.Scan() {
				scanJobs, err = scanBody(event, loc, scanner)
				if err != nil {
					return nil, err
				}
			}

			return scanJobs, scanner.Err()
		})
	}
	return js
}

func scanBody(event *beat.Event, loc string, scanner *bufio.Scanner) ([]jobs.Job, error) {
	unmarshalled := common.MapStr{}
	err := json.Unmarshal(scanner.Bytes(), &unmarshalled)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal chunk: %w / %s", err, scanner.Text())
	}

	eventext.MergeEventFields(event, common.MapStr{"observer": common.MapStr{"geo": common.MapStr{"name": loc}}})
	ts, _ := unmarshalled["@timestamp"]
	parsedTs, err := time.Parse(time.RFC3339, ts.(string))
	if err != nil {
		return nil, fmt.Errorf("could not parse event time from srv: %w", err)
	}
	event.Timestamp = parsedTs
	// Without this serialization breaks with dupe TS
	unmarshalled.Delete("@timestamp")

	eventext.MergeEventFields(event, unmarshalled)

	if scanner.Scan() {
		return []jobs.Job{func(event *beat.Event) ([]jobs.Job, error) {
			return scanBody(event, loc, scanner)
		}}, nil
	}

	return nil, nil
}

