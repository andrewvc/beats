// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package browser

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"sync"

	"github.com/elastic/beats/v7/x-pack/heartbeat/monitors/browser/source"

	"github.com/elastic/beats/v7/heartbeat/monitors/jobs"
	"github.com/elastic/beats/v7/heartbeat/monitors/plugin"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/beats/v7/x-pack/heartbeat/monitors/browser/synthexec"
)

func init() {
	plugin.Register("browser", create, "synthetic", "synthetics/synthetic")
}

var showExperimentalOnce = sync.Once{}

var NotSyntheticsCapableError = fmt.Errorf("synthetic monitors cannot be created outside the official elastic docker image")

func create(name string, cfg *common.Config) (p plugin.Plugin, err error) {
	// We don't want users running synthetics in environments that don't have the required GUI libraries etc, so we check
	// this flag. When we're ready to support the many possible configurations of systems outside the docker environment
	// we can remove this check.
	if os.Getenv("ELASTIC_SYNTHETICS_CAPABLE") != "true" {
		return plugin.Plugin{}, NotSyntheticsCapableError
	}

	showExperimentalOnce.Do(func() {
		logp.Info("Synthetic monitor detected! Please note synthetic monitors are an experimental unsupported feature!")
	})

	parsedConfig := DefaultConfig()
	err = cfg.Unpack(parsedConfig)
	if err != nil {
		return plugin.Plugin{}, fmt.Errorf("could not unpack config: %s", err)
	}

	return run(cfg)
}

type cloudBody struct {
	Source source.Source          `json:"source"`
	Params map[string]interface{} `json:"params"`
}

func run(cfg *common.Config) (p plugin.Plugin, err error) {
	curUser, err := user.Current()
	if err != nil {
		return plugin.Plugin{}, fmt.Errorf("could not determine current user for script monitor %w: ", err)
	}
	if curUser.Uid == "0" {
		return plugin.Plugin{}, fmt.Errorf("script monitors cannot be run as root! Current UID is %s", curUser.Uid)
	}

	ss, err := NewSuite(cfg)
	if err != nil {
		return plugin.Plugin{}, err
	}

	extraArgs := []string{}
	if ss.suiteCfg.Sandbox {
		extraArgs = append(extraArgs, "--sandbox")
	}

	if ss.suiteCfg.DryRun {
		extraArgs = append(extraArgs, "--dry-run")
	}

	var js []jobs.Job
	close := ss.Close
	if ss.suiteCfg.Cloud != nil {
		for locName, locUrl := range ss.suiteCfg.Cloud.Locations {
			j := synthexec.CloudJob(context.TODO(), ss.CloudExec, locName, locUrl)
			js = append(js, j)
		}
		close = func() error { return nil }
	} else if src, ok := ss.InlineSource(); ok {
		j := synthexec.InlineJourneyJob(context.TODO(), src, ss.Params(), extraArgs...)
		js = append(js, j)
	} else {
		j := func(event *beat.Event) ([]jobs.Job, error) {
			err := ss.Fetch()
			if err != nil {
				return nil, fmt.Errorf("could not fetch for suite job: %w", err)
			}
			sj, err := synthexec.SuiteJob(context.TODO(), ss.Workdir(), ss.Params(), extraArgs...)
			if err != nil {
				return nil, err
			}
			return sj(event)
		}
		js = append(js, j)
	}

	return plugin.Plugin{
		Jobs:      js,
		DoClose:   close,
		Endpoints: 1,
	}, nil
}
