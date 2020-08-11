// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package wrappers

import (
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/heartbeat/eventext"
	"github.com/elastic/beats/v7/heartbeat/look"
	"github.com/elastic/beats/v7/heartbeat/monitors/jobs"
	"github.com/elastic/beats/v7/heartbeat/monitors/stdfields"
	"github.com/elastic/beats/v7/heartbeat/scheduler/schedule"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"
)


// WrapCommon applies the common wrappers that all monitor jobs get.
func WrapCommon(js []jobs.Job, stdMonFields stdfields.StdMonitorFields) []jobs.Job {
	statusBlockTracker := newStatusBlockTracker()
	return jobs.WrapAllSeparately(
		jobs.WrapAll(
			js,
			addMonitorStatus,
			addMonitorDuration,
		), func() jobs.JobWrapper {
			return addMonitorMeta(stdMonFields, len(js) > 1, statusBlockTracker)
		}, func() jobs.JobWrapper {
			return makeAddSummary(statusBlockTracker)
		})
}

// addMonitorMeta adds the id, name, and type fields to the monitor.
func addMonitorMeta(stdMonFields stdfields.StdMonitorFields, isMulti bool, statusBlockTracker *monitorStateTracker) jobs.JobWrapper {
	return func(job jobs.Job) jobs.Job {
		return func(event *beat.Event) ([]jobs.Job, error) {
			started := time.Now()
			cont, e := job(event)
			thisID := stdMonFields.ID

			if isMulti {
				url, err := event.GetValue("url.full")
				if err != nil {
					logp.Error(errors.Wrap(err, "Mandatory url.full key missing!"))
					url = "n/a"
				}
				urlHash, _ := hashstructure.Hash(url, nil)
				thisID = fmt.Sprintf("%s-%x", stdMonFields.ID, urlHash)
			}

			status, _ := event.Fields.GetValue("monitor.status")
			var trackerStatus stateStatus
			if status == "down" {
				trackerStatus = StatusDown
			} else {
				trackerStatus = StatusUp
			}
			ip, _ := event.Fields.GetValue("monitor.ip")
			trackerId := fmt.Sprintf("%s-%s", stdMonFields.ID, ip)

			sb := statusBlockTracker.getID(trackerId, trackerStatus)

			fieldsToMerge := common.MapStr{
				"monitor": common.MapStr{
					"id":       thisID,
					"name":     stdMonFields.Name,
					"type":     stdMonFields.Type,
					"timespan": timespan(started, stdMonFields.Schedule, stdMonFields.Timeout),
					"status_block": sb,
				},
			}

			if stdMonFields.ServiceName != "" {
				fieldsToMerge["service"] = common.MapStr{
					"name": stdMonFields.ServiceName,
				}
			}

			eventext.MergeEventFields(event, fieldsToMerge)

			return cont, e
		}
	}
}

func timespan(started time.Time, sched *schedule.Schedule, timeout time.Duration) common.MapStr {
	maxEnd := sched.Next(started)

	if maxEnd.Sub(started) < timeout {
		maxEnd = started.Add(timeout)
	}

	return common.MapStr{
		"gte": started,
		"lt":  maxEnd,
	}
}

// addMonitorStatus wraps the given Job's execution such that any error returned
// by the original Job will be set as a field. The original error will not be
// passed through as a return value. Errors may still be present but only if there
// is an actual error wrapping the error.
func addMonitorStatus(origJob jobs.Job) jobs.Job {
	return func(event *beat.Event) ([]jobs.Job, error) {
		cont, err := origJob(event)

		fields := common.MapStr{
			"monitor": common.MapStr{
				"status": look.Status(err),
			},
		}
		if err != nil {
			fields["error"] = look.Reason(err)
		}
		eventext.MergeEventFields(event, fields)
		return cont, nil
	}
}

// addMonitorDuration executes the given Job, checking the duration of its run.
func addMonitorDuration(job jobs.Job) jobs.Job {
	return func(event *beat.Event) ([]jobs.Job, error) {
		start := time.Now()

		cont, err := job(event)

		if event != nil {
			eventext.MergeEventFields(event, common.MapStr{
				"monitor": common.MapStr{
					"duration": look.RTT(time.Since(start)),
				},
			})
			event.Timestamp = start
		}

		return cont, err
	}
}

const FlappingThreshold time.Duration = time.Minute

const (
	StatusUp stateStatus = iota
	StatusDown
	StatusMixed
)

type stateStatus int8

type historicalStatus struct {
	ts     time.Time
	status stateStatus
}

type monitorState struct {
	startedAt   time.Time
	status      stateStatus
	flapHistory []historicalStatus
}

func (state *monitorState) isFlapping() bool {
	return len(state.flapHistory) > 0
}

func (state *monitorState) isStateStillStable(currentStatus stateStatus) bool {
	return state.status == currentStatus && state.isFlapping()
}

func (state *monitorState) flapCompute(currentStatus stateStatus) bool {
	state.flapHistory = append(state.flapHistory, historicalStatus{time.Now(), state.status})
	state.status = currentStatus

	// Figure out which values are old enough that we can discard them for our calculation
	cutOff := time.Now().Add(-FlappingThreshold)
	discardIndex := -1
	for idx, hs := range state.flapHistory {
		if hs.ts.Before(cutOff) {
			discardIndex = idx
		} else {
			break
		}
	}
	// Do the discarding
	if discardIndex != -1 {
		state.flapHistory = state.flapHistory[discardIndex+1:]
	}

	// Check to see if we are no longer flapping, and if so clear flap history
	for _, hs := range state.flapHistory {
		if hs.status != currentStatus {
			return false
		}
	}
	return true
}

func NewMonitorState(currentStatus stateStatus) *monitorState {
	return &monitorState{
		startedAt: time.Now(),
		status:    currentStatus,
	}
}



func newStatusBlockTracker() *monitorStateTracker {
	return &monitorStateTracker{
		states: map[string]*monitorState{},
		mtx:    sync.Mutex{},
	}
}

type monitorStateTracker struct {
	states map[string]*monitorState
	mtx    sync.Mutex
}

func (mst *monitorStateTracker) get(monitorId string, currentStatus stateStatus) (state *monitorState) {
	if state, ok := mst.states[monitorId]; ok {
		if state.isFlapping() {
			// Check to see if there's still an ongoing flap after recording
			// the new status
			if state.flapCompute(currentStatus) {
				return state
			} else {
				state = NewMonitorState(currentStatus)
				mst.states[monitorId] = state
				return state
			}
		} else if state.status == currentStatus {
			// The state is stable, no changes needed
			return state
		} else if state.startedAt.After(time.Now().Add(-FlappingThreshold)) {
			state.flapCompute(currentStatus) // record the new state to the flap history
			return state
		}
	}

	// No previous state, so make a new one
	state = NewMonitorState(currentStatus)
	mst.states[monitorId] = state
	return state
}

func (mst *monitorStateTracker) getID(monitorId string, currentStatus stateStatus) time.Time {
	mst.mtx.Lock()
	defer mst.mtx.Unlock()
	return mst.get(monitorId, currentStatus).startedAt
}

// makeAddSummary summarizes the job, adding the `summary` field to the last event emitted.
func makeAddSummary(sb *monitorStateTracker) jobs.JobWrapper {
	// This is a tricky method. The way this works is that we track the state across jobs in the
	// state struct here.
	state := struct {
		mtx        sync.Mutex
		remaining  uint16
		up         uint16
		down       uint16
		checkGroup string
		generation uint64
	}{
		mtx: sync.Mutex{},
	}
	// Note this is not threadsafe, must be called from a mutex
	resetState := func() {
		state.remaining = 1
		state.up = 0
		state.down = 0
		state.generation++
		u, err := uuid.NewV1()
		if err != nil {
			panic(fmt.Sprintf("cannot generate UUIDs on this system: %s", err))
		}
		state.checkGroup = u.String()
	}
	resetState()

	return func(job jobs.Job) jobs.Job {
		return func(event *beat.Event) ([]jobs.Job, error) {
			cont, err := job(event)
			state.mtx.Lock()
			defer state.mtx.Unlock()

			// If the event is cancelled we don't record it as being either up or down since
			// we discard the event anyway.
			if !eventext.IsEventCancelled(event) {
				// After each job
				eventStatus, _ := event.GetValue("monitor.status")
				if eventStatus == "up" {
					state.up++
				} else {
					state.down++
				}
			}

			// No error check needed here
			event.PutValue("monitor.check_group", state.checkGroup)

			// Adjust the total remaining to account for new continuations
			state.remaining += uint16(len(cont))
			// Reduce total remaining to account for the just executed job
			state.remaining--

			// After last job
			if state.remaining == 0 {
				monitorId, _ := event.GetValue("monitor.id")
				var trackerStatus stateStatus
				if state.down == 0 {
					trackerStatus = StatusUp
				} else if state.up > 0 {
					trackerStatus = StatusMixed
				} else {
					trackerStatus = StatusDown
				}
				monitorIdString, _ := monitorId.(string)
				cssId := sb.getID(monitorIdString, trackerStatus)
				eventext.MergeEventFields(event, common.MapStr{
					"summary": common.MapStr{
						"continuous_status_segment": cssId,
						"up":                        state.up,
						"down":                      state.down,
					},
				})
				resetState()
			}

			return cont, err
		}
	}
}
