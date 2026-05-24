package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseCustomCommand(t *testing.T) {
	root := filepath.Join("srv", "svc") // platform-neutral root for Join
	j := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	tests := []struct {
		name       string
		cmdStr     string
		wantScript string
		wantArgs   []string
	}{
		{
			name:       "empty — no script declared",
			cmdStr:     "",
			wantScript: "",
			wantArgs:   nil,
		},
		{
			name:       "path only — no args",
			cmdStr:     "bin/runCMD.sh",
			wantScript: j("bin/runCMD.sh"),
			wantArgs:   nil,
		},
		{
			name:       "shared script — start arg",
			cmdStr:     "bin/runCMD.sh start",
			wantScript: j("bin/runCMD.sh"),
			wantArgs:   []string{"start"},
		},
		{
			name:       "shared script — stop arg",
			cmdStr:     "bin/runCMD.sh stop",
			wantScript: j("bin/runCMD.sh"),
			wantArgs:   []string{"stop"},
		},
		{
			name:       "multiple args",
			cmdStr:     "bin/manage.sh start --env dev",
			wantScript: j("bin/manage.sh"),
			wantArgs:   []string{"start", "--env", "dev"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotScript, gotArgs := parseCustomCommand(root, tc.cmdStr)
			if gotScript != tc.wantScript {
				t.Errorf("script: got %q, want %q", gotScript, tc.wantScript)
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args: got %v, want %v", gotArgs, tc.wantArgs)
			}
		})
	}
}

func TestPickScript(t *testing.T) {
	tests := []struct {
		name    string
		scripts []string
		isStart bool
		want    string
	}{
		{name: "empty list", scripts: nil, isStart: true, want: ""},
		{name: "start: runCMD picked over stopCMD", scripts: []string{"runCMD.sh", "stopCMD.sh"}, isStart: true, want: "runCMD.sh"},
		{name: "stop: stopCMD picked", scripts: []string{"runCMD.sh", "stopCMD.sh"}, isStart: false, want: "stopCMD.sh"},
		{name: "start: single non-stop script used unconditionally", scripts: []string{"manage.sh"}, isStart: true, want: "manage.sh"},
		{name: "stop: no stop keyword → blank", scripts: []string{"manage.sh"}, isStart: false, want: ""},
		{name: "start: prefers 'run' over generic name", scripts: []string{"service.sh", "runSvc.sh"}, isStart: true, want: "runSvc.sh"},
		{name: "start: shared script (only one, has 'run')", scripts: []string{"runCMD.sh"}, isStart: true, want: "runCMD.sh"},
		{name: "stop: shared script name containing stop", scripts: []string{"runCMD.sh"}, isStart: false, want: ""},
		{name: "start: ambiguous two non-stop scripts → blank", scripts: []string{"launch.sh", "service.sh"}, isStart: true, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickScript(tc.scripts, tc.isStart)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
