package rserver

import (
	"encoding/json"
	"fmt"
	"github.com/elastic/beats/v7/heartbeat/eventext"
	"github.com/elastic/beats/v7/heartbeat/monitors/jobs"
	"github.com/elastic/beats/v7/heartbeat/monitors/plugin"
	"github.com/elastic/beats/v7/heartbeat/monitors/stdfields"
	"github.com/elastic/beats/v7/heartbeat/monitors/wrappers"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
	"net/http"
	"sync"
)

func init() {
	plugin.Register("rserver", create)
}

type runResult struct {
	UserId string `json:"user_id"`
	RuntimeNanos int64 `json:"runtime_nanos"`
	Type string `json:"type"`
}

type rserverConfig struct {
	Users map[string]string `config:"users"`
}

func create(name string, cfg *common.Config) (p plugin.Plugin, err error) {
	config := &rserverConfig{}
	err = cfg.Unpack(config)
	if err != nil {
		return plugin.Plugin{}, err
	}

	results := make(chan runResult)

	go func() {
		startServer(":8080", results, config.Users)
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

func startServer(addr string, results chan runResult, users map[string]string) {
	logp.Info("Starting rserver")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		handleErr := func (context string, err error) {
			writer.WriteHeader(400)
			writer.Write([]byte(fmt.Sprintf("unexpected error [%s] %s", context, err)))
		}

		user, password, hasPassword := request.BasicAuth()

		if !hasPassword {
			handleErr("auth", fmt.Errorf("no password specified"))
			return
		}

		if userPass, ok := users[user]; !ok || password != userPass {
			handleErr("auth", fmt.Errorf("could not authorize user"))
			logp.Warn("Users %v", users)
			return
		}

		parsed := &common.MapStr{}
		decodeErr := json.NewDecoder(request.Body).Decode(&parsed)
		if decodeErr != nil {
			handleErr("decoding", decodeErr)
			return
		}

		config, err := common.NewConfigFrom(parsed)
		if err != nil {
			handleErr("reading config", err)
			return
		}

		fields, err := stdfields.ConfigToStdMonitorFields(config)
		if err != nil {
			handleErr("stdfields", err)
			return
		}

		p, ok := plugin.GlobalPluginsReg.Get(fields.Type)
		if !ok {
			handleErr("plugin reg", fmt.Errorf("could not find plugin for %s", fields.Type))
			return
		}

		builtPlugin, err := p.Builder(fields.Type, config)
		if err != nil {
			handleErr("plugin build", err)
			return
		}

		flusher, ok := writer.(http.Flusher)
		if !ok {
			panic("not a flusher!")
		}

		wg := sync.WaitGroup{}
		writeMtx := sync.Mutex{}
		var runRecursive func(j jobs.Job)
		runRecursive = func(j jobs.Job) {
			e := &beat.Event{Fields: common.MapStr{}}
			conts, _ := j(e)

			out := e.Fields
			out.Put("@timestamp", e.Timestamp)
			marshalled, err := json.Marshal(out)
			marshalled = append(marshalled, []byte("\n")...)
			if err != nil {
				logp.Warn("Could not marshall: %s", err)
			}

			writeMtx.Lock()
			_, err = writer.Write(marshalled)
			if err != nil {
				logp.Warn("Could not write chunk %s", err)
			}
			flusher.Flush()
			writeMtx.Unlock()

			for _, cj := range conts {
				wg.Add(1)
				go runRecursive(cj)
			}

			wg.Done()
		}
		for _, j := range wrappers.WrapCommon(builtPlugin.Jobs, fields) {
			wg.Add(1)
			runRecursive(j)
		}
		wg.Wait()

		results <- runResult{
			Type: fields.Type,
			UserId: user,
		}
	})
	err := http.ListenAndServe(addr, mux)
	if err != nil {
		panic(fmt.Sprintf("could not start rserver mux: %s", err))
	}
}
