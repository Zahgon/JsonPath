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

package jsonpath

import "fmt"

// Option tweaks how paths are evaluated.
type Option int

// The evaluation options.
const (
	// DefaultPathLeafToNull returns null for a missing leaf instead of failing or skipping it.
	DefaultPathLeafToNull Option = iota
	// AlwaysReturnList makes a definite path return its result inside a list.
	AlwaysReturnList
	// AsPathList returns the normalised paths of the results instead of their values.
	AsPathList
	// SuppressExceptions returns null or an empty list instead of failing on a missing path.
	SuppressExceptions
	// RequireProperties fails on a missing property even where the path is indefinite.
	RequireProperties
)

var optionNames = [...]string{
	DefaultPathLeafToNull: "DEFAULT_PATH_LEAF_TO_NULL",
	AlwaysReturnList:      "ALWAYS_RETURN_LIST",
	AsPathList:            "AS_PATH_LIST",
	SuppressExceptions:    "SUPPRESS_EXCEPTIONS",
	RequireProperties:     "REQUIRE_PROPERTIES",
}

// String returns the option's constant name, as DEFAULT_PATH_LEAF_TO_NULL.
func (o Option) String() string {
	if o >= 0 && int(o) < len(optionNames) {
		return optionNames[o]
	}
	return fmt.Sprintf("Option(%d)", int(o))
}

// ParseOption returns the option named name (DEFAULT_PATH_LEAF_TO_NULL, ALWAYS_RETURN_LIST, AS_PATH_LIST,
// SUPPRESS_EXCEPTIONS or REQUIRE_PROPERTIES).
func ParseOption(name string) (Option, bool) {
	for i, n := range optionNames {
		if n == name {
			return Option(i), true
		}
	}
	return 0, false
}

// EvaluationContinuation tells the evaluation whether to go on after a result.
type EvaluationContinuation int

// Continuations an EvaluationListener returns.
const (
	Continue EvaluationContinuation = iota
	Abort
)

// FoundResult describes one result as it is found: its index among the results, its path and its value.
type FoundResult struct {
	Index int
	Path  string
	Value any
}

// EvaluationListener is told about every result as it is found and can stop the evaluation.
type EvaluationListener func(FoundResult) EvaluationContinuation

// Configuration holds the options and evaluation listeners of an evaluation. The JSON model is always json-smart's.
// A Configuration is immutable; the With methods return new ones.
type Configuration struct {
	options   [len(optionNames)]bool
	listeners []EvaluationListener
}

// NewConfiguration returns a configuration with the given options.
func NewConfiguration(options ...Option) *Configuration {
	c := &Configuration{}
	for _, o := range options {
		c.options[o] = true
	}
	return c
}

// DefaultConfiguration returns a configuration without options.
func DefaultConfiguration() *Configuration {
	return NewConfiguration()
}

// Has reports whether the option is set.
func (c *Configuration) Has(o Option) bool {
	return c.options[o]
}

// Options returns the options that are set, in declaration order.
func (c *Configuration) Options() []Option {
	var out []Option
	for i, set := range c.options {
		if set {
			out = append(out, Option(i))
		}
	}
	return out
}

// WithOptions returns a copy of c with the given options added.
func (c *Configuration) WithOptions(options ...Option) *Configuration {
	n := &Configuration{options: c.options, listeners: c.listeners}
	for _, o := range options {
		n.options[o] = true
	}
	return n
}

// WithListeners returns a copy of c whose evaluation listeners are replaced by listeners.
func (c *Configuration) WithListeners(listeners ...EvaluationListener) *Configuration {
	return &Configuration{options: c.options, listeners: append([]EvaluationListener(nil), listeners...)}
}

// Listeners returns the evaluation listeners.
func (c *Configuration) Listeners() []EvaluationListener {
	return c.listeners
}
