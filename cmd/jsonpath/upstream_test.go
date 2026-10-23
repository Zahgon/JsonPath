// Copyright 2011 the original author or authors (Jayway JsonPath, ported to Go).
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// upstreamCases are the paths and filters of the upstream JUnit suites, with the output of the original command
// recorded for each.
type upstreamCases struct {
	Documents map[string]string `json:"documents"`
	Cases     []struct {
		Suite    string   `json:"suite"`
		Args     []string `json:"args"`
		Document string   `json:"document"`
		Stdout   string   `json:"stdout"`
		Stderr   string   `json:"stderr"`
		Status   int      `json:"status"`
	} `json:"cases"`
}

func TestUpstreamSuites(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream.json")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}
	var cases upstreamCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decoding testdata: %v", err)
	}
	for _, c := range cases.Cases {
		t.Run(c.Suite+"/"+strings.Join(c.Args, " "), func(t *testing.T) {
			out, errText, status := run(c.Args, strings.NewReader(cases.Documents[c.Document]))
			if out != c.Stdout || errText != c.Stderr || status != c.Status {
				t.Errorf("run(%q) = (%q, %q, %d), want (%q, %q, %d)", c.Args, out, errText, status, c.Stdout, c.Stderr,
					c.Status)
			}
		})
	}
}
