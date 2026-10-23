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

import (
	"errors"
	"fmt"

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/smartjson"
)

// Kind classifies an Error. The JsonPath kinds form JsonPath's exception hierarchy: every one of them is a
// KindJSONPath, and KindPathNotFound is also a KindInvalidPath. The remaining kinds are the general runtime failures
// JsonPath lets through for some inputs (a bad number in a filter, an index outside a list, ...).
type Kind int

// Error kinds, each named after the exception it stands for.
const (
	KindJSONPath               Kind = iota // JsonPathException
	KindInvalidPath                        // InvalidPathException
	KindPathNotFound                       // PathNotFoundException
	KindInvalidJSON                        // InvalidJsonException
	KindInvalidModification                // InvalidModificationException
	KindIllegalArgument                    // IllegalArgumentException
	KindIllegalState                       // IllegalStateException
	KindNumberFormat                       // NumberFormatException
	KindIndexOutOfBounds                   // IndexOutOfBoundsException
	KindStringIndexOutOfBounds             // StringIndexOutOfBoundsException
	KindNullPointer                        // NullPointerException
	KindClassCast                          // ClassCastException
	KindUnsupportedOperation               // UnsupportedOperationException
)

var kindClasses = map[Kind]string{
	KindJSONPath:               "com.jayway.jsonpath.JsonPathException",
	KindInvalidPath:            "com.jayway.jsonpath.InvalidPathException",
	KindPathNotFound:           "com.jayway.jsonpath.PathNotFoundException",
	KindInvalidJSON:            "com.jayway.jsonpath.InvalidJsonException",
	KindInvalidModification:    "com.jayway.jsonpath.InvalidModificationException",
	KindIllegalArgument:        "java.lang.IllegalArgumentException",
	KindIllegalState:           "java.lang.IllegalStateException",
	KindNumberFormat:           "java.lang.NumberFormatException",
	KindIndexOutOfBounds:       "java.lang.IndexOutOfBoundsException",
	KindStringIndexOutOfBounds: "java.lang.StringIndexOutOfBoundsException",
	KindNullPointer:            "java.lang.NullPointerException",
	KindClassCast:              "java.lang.ClassCastException",
	KindUnsupportedOperation:   "java.lang.UnsupportedOperationException",
}

// Error is a failure of a JsonPath operation. Message is the exception message; a failure without a message has
// HasMessage false.
type Error struct {
	Kind       Kind
	Message    string
	HasMessage bool
}

// Error returns the exception name followed by ": " and the message, or the name alone when there is no message.
func (e *Error) Error() string {
	if !e.HasMessage {
		return e.Name()
	}
	return e.Name() + ": " + e.Message
}

// ClassName returns the fully qualified name of the exception class the error stands for.
func (e *Error) ClassName() string {
	return kindClasses[e.Kind]
}

// Name returns the simple name of the exception class the error stands for.
func (e *Error) Name() string {
	name := e.ClassName()
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return name
}

// IsJSONPath reports whether the error belongs to JsonPath's own exception hierarchy.
func (e *Error) IsJSONPath() bool {
	return e.Kind <= KindInvalidModification
}

// Is matches the sentinel errors of the kinds this error belongs to.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok || t.HasMessage {
		return false
	}
	switch t.Kind {
	case e.Kind:
		return true
	case KindJSONPath:
		return e.IsJSONPath()
	case KindInvalidPath:
		return e.Kind == KindPathNotFound
	}
	return false
}

// javaString renders the error as Java's Throwable.toString, the message of an exception that wraps it.
func (e *Error) javaString() string {
	if !e.HasMessage {
		return e.ClassName()
	}
	return e.ClassName() + ": " + e.Message
}

// Sentinels for errors.Is.
var (
	ErrJSONPath            = &Error{Kind: KindJSONPath}
	ErrInvalidPath         = &Error{Kind: KindInvalidPath}
	ErrPathNotFound        = &Error{Kind: KindPathNotFound}
	ErrInvalidJSON         = &Error{Kind: KindInvalidJSON}
	ErrInvalidModification = &Error{Kind: KindInvalidModification}
	ErrIllegalArgument     = &Error{Kind: KindIllegalArgument}
)

func newError(kind Kind, format string, args ...any) *Error {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &Error{Kind: kind, Message: msg, HasMessage: true}
}

func invalidPath(format string, args ...any) *Error {
	return newError(KindInvalidPath, format, args...)
}

func pathNotFound(format string, args ...any) *Error {
	return newError(KindPathNotFound, format, args...)
}

// wrap returns an error of kind whose message is the Java string of cause, as an exception built from a cause.
func wrap(kind Kind, cause error) *Error {
	return &Error{Kind: kind, Message: causeString(cause), HasMessage: true}
}

func causeString(cause error) string {
	var e *Error
	var nf *java.NumberFormatError
	var pe *smartjson.ParseError
	switch {
	case errors.As(cause, &e):
		return e.javaString()
	case errors.As(cause, &nf):
		return "java.lang.NumberFormatException: " + nf.Message
	case errors.As(cause, &pe):
		return pe.String()
	}
	return cause.Error()
}

// isKind reports whether err is an *Error of one of the kinds its sentinel matches.
func isKind(err error, sentinel *Error) bool {
	return errors.Is(err, sentinel)
}

// asError converts any failure into an *Error, keeping the kind of a number-format failure.
func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var nf *java.NumberFormatError
	if errors.As(err, &nf) {
		return newError(KindNumberFormat, "%s", nf.Message)
	}
	return newError(KindJSONPath, "%s", err.Error())
}
