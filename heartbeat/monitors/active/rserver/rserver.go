package rserver

import (
	"encoding/json"
	"fmt"
	"github.com/elastic/beats/v7/heartbeat/eventext"
	"github.com/elastic/beats/v7/heartbeat/monitors/jobs"
	"github.com/elastic/beats/v7/heartbeat/monitors/plugin"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
	"net/http"
)

func init() {
	plugin.Register("rserver", create)
}

type runResult struct {
	UserId string `json:"user_id"`
	RuntimeNanos int64 `json:"runtime_nanos"`
	Type string `json:"type"`
}

func create(name string, cfg *common.Config) (p plugin.Plugin, err error) {
	results := make(chan runResult)

	go func() {
		startServer(":5678")
	}()

	return plugin.Plugin{
		Endpoints: 0,
		Close: nil,
		Jobs: []jobs.Job{readResultFrom(results)},
	}, nil
}

func readResultFrom(results chan runResult) jobs.Job {
	return func(event *beat.Event) ([]jobs.Job, error) {
		err := readResult(event, results)
		return []jobs.Job{readResultFrom(results)}, err
	}
}
func readResult (event *beat.Event, results chan runResult) (error) {
	r := <- results
	eventext.MergeEventFields(event, common.MapStr{"run": r})
	return nil
}

func startServer(addr string) {
	logp.Info("Starting rserver")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		parsed := &common.MapStr{}
		decodeErr := json.NewDecoder(request.Body).Decode(&parsed)
		if decodeErr != nil {
			logp.Warn("Could not decode %v", decodeErr)
		}

		out, err := json.Marshal(parsed)
		if err != nil {
			logp.Warn("could not marshal decoded")
		}
		writer.Write(out)
	})
	err := http.ListenAndServe(addr, mux)
	if err != nil {
		panic(fmt.Sprintf("could not start rserver mux: %s", err))
	}
}
