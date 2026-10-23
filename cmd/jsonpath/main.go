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

// Command jsonpath runs one JsonPath operation on one JSON document read from standard input.
//
// Usage:
//
//	jsonpath [OPTIONS] COMMAND ARGS...
//
// Options, before the command:
//
//	--option NAME    enable an evaluation option (DEFAULT_PATH_LEAF_TO_NULL, ALWAYS_RETURN_LIST, AS_PATH_LIST,
//	                 SUPPRESS_EXCEPTIONS, REQUIRE_PROPERTIES); repeatable
//	--filter FILTER  a filter for the next [?] placeholder of the path; repeatable, in order
//	--limit N        read only: stop after N results
//	--typed          print values in typed notation instead of JSON
//
// Commands:
//
//	read PATH                     print the value the path selects
//	set PATH VALUE                replace the selected values, then print the result and the document
//	put PATH KEY VALUE            set KEY in the selected objects, then print the result and the document
//	add PATH VALUE                append to the selected arrays, then print the result and the document
//	delete PATH                   remove the selected values, then print the result and the document
//	renameKey PATH OLDKEY NEWKEY  rename a key of the selected objects, then print the result and the document
//	compile PATH                  print the normalised path and whether it is definite (no standard input)
//	filter FILTER                 print the normalised filter (no standard input)
//
// The document (UTF-8) and every VALUE are parsed as permissive JSON. Each value printed is one line of compact
// JSON, or of typed notation: null; bool:V; int:V, long:V, bigint:V, double:V, float:V and bigdec:V; string:S with
// S the JSON string; [E,...] for an array; {K:E,...} for an object; other:NAME for anything else.
//
// The exit status is 0 on success; 1 when the operation fails with a JsonPath error or an illegal argument
// (standard error: the error name, ": " and the message when there is one); 2 for a usage error (standard error:
// "usage: " and the reason); 3 for any other failure (standard error: "internal error: " and the failure's name).
// Nothing is printed to standard output unless the exit status is 0.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/json-path/JsonPath/internal/java"
	"github.com/json-path/JsonPath/jsonpath"
	"github.com/json-path/JsonPath/smartjson"
)

func main() {
	out, errText, status := run(os.Args[1:], os.Stdin)
	if _, err := io.WriteString(os.Stdout, out); err != nil {
		os.Exit(3)
	}
	if _, err := io.WriteString(os.Stderr, errText); err != nil {
		os.Exit(3)
	}
	os.Exit(status)
}

// usageError is a malformed command line.
type usageError string

func (e usageError) Error() string {
	return string(e)
}

// run executes one command and returns what goes to standard output and standard error, and the exit status.
func run(args []string, stdin io.Reader) (string, string, int) {
	var out strings.Builder
	err := execute(args, stdin, &out)
	if err == nil {
		return out.String(), "", 0
	}
	var usage usageError
	if errors.As(err, &usage) {
		return "", "usage: " + string(usage) + "\n", 2
	}
	var jerr *jsonpath.Error
	if errors.As(err, &jerr) {
		// NumberFormatException is an IllegalArgumentException.
		if jerr.IsJSONPath() || jerr.Kind == jsonpath.KindIllegalArgument || jerr.Kind == jsonpath.KindNumberFormat {
			return "", jerr.Error() + "\n", 1
		}
		return "", "internal error: " + jerr.Name() + "\n", 3
	}
	return "", "internal error: " + err.Error() + "\n", 3
}

type invocation struct {
	options []jsonpath.Option
	filters []string
	limit   *int
	typed   bool
	command string
	args    []string
}

var arities = map[string]int{
	"read": 1, "delete": 1, "compile": 1, "filter": 1,
	"set": 2, "add": 2,
	"put": 3, "renameKey": 3,
}

func parseArgs(args []string) (*invocation, error) {
	inv := &invocation{}
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "--") {
		flag := args[i]
		if flag == "--typed" {
			inv.typed = true
			i++
			continue
		}
		if flag != "--option" && flag != "--filter" && flag != "--limit" {
			return nil, usageError("unknown flag " + flag)
		}
		if i+1 >= len(args) {
			return nil, usageError(flag + " needs a value")
		}
		value := args[i+1]
		i += 2
		switch flag {
		case "--option":
			o, ok := jsonpath.ParseOption(value)
			if !ok {
				return nil, usageError("unknown option " + value)
			}
			inv.options = append(inv.options, o)
		case "--filter":
			inv.filters = append(inv.filters, value)
		default:
			n, err := java.ParseInt(value)
			if err != nil {
				return nil, usageError("bad limit " + value)
			}
			limit := int(n)
			inv.limit = &limit
		}
	}
	if i >= len(args) {
		return nil, usageError("missing command")
	}
	inv.command = args[i]
	i++
	arity, ok := arities[inv.command]
	if !ok {
		return nil, usageError("unknown command " + inv.command)
	}
	if len(args)-i != arity {
		noun := "arguments"
		if arity == 1 {
			noun = "argument"
		}
		return nil, usageError(fmt.Sprintf("%s takes %d %s", inv.command, arity, noun))
	}
	if inv.limit != nil && inv.command != "read" {
		return nil, usageError("--limit applies to read only")
	}
	inv.args = args[i:]
	return inv, nil
}

func execute(args []string, stdin io.Reader, out *strings.Builder) error {
	inv, err := parseArgs(args)
	if err != nil {
		return err
	}
	a := inv.args
	if inv.command == "filter" {
		f, err := jsonpath.ParseFilter(a[0])
		if err != nil {
			return err
		}
		out.WriteString(f.String() + "\n")
		return nil
	}
	predicates := make([]jsonpath.Predicate, 0, len(inv.filters))
	for _, text := range inv.filters {
		f, err := jsonpath.ParseFilter(text)
		if err != nil {
			return err
		}
		predicates = append(predicates, f)
	}
	if inv.command == "compile" {
		p, err := jsonpath.Compile(a[0], predicates...)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n%t\n", p, p.IsDefinite())
		return nil
	}

	conf := jsonpath.NewConfiguration(inv.options...)
	text, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	document, err := jsonpath.Parse(strings.ToValidUTF8(string(text), "�"))
	if err != nil {
		return err
	}
	if inv.command == "read" {
		ctx, err := jsonpath.NewDocumentContext(document, conf)
		if err != nil {
			return err
		}
		if inv.limit != nil {
			ctx = ctx.Limit(*inv.limit)
		}
		result, err := ctx.Read(a[0], predicates...)
		if err != nil {
			return err
		}
		out.WriteString(render(result, inv.typed) + "\n")
		return nil
	}

	path, err := jsonpath.Compile(a[0], predicates...)
	if err != nil {
		return err
	}
	var result any
	switch inv.command {
	case "set", "add":
		value, err := jsonpath.Parse(a[1])
		if err != nil {
			return err
		}
		if inv.command == "set" {
			result, err = path.Set(document, value, conf)
		} else {
			result, err = path.Add(document, value, conf)
		}
		if err != nil {
			return err
		}
	case "put":
		value, err := jsonpath.Parse(a[2])
		if err != nil {
			return err
		}
		if result, err = path.Put(document, a[1], value, conf); err != nil {
			return err
		}
	case "delete":
		if result, err = path.Delete(document, conf); err != nil {
			return err
		}
	default:
		if result, err = path.RenameKey(document, a[1], a[2], conf); err != nil {
			return err
		}
	}
	out.WriteString(render(result, inv.typed) + "\n")
	out.WriteString(render(document, inv.typed) + "\n")
	return nil
}

func render(v any, typed bool) string {
	if !typed {
		return smartjson.Marshal(v, smartjson.StyleCompact)
	}
	var b strings.Builder
	writeTyped(&b, v)
	return b.String()
}

// writeTyped prints a value in typed notation, naming the Java type json-smart uses for each number.
func writeTyped(b *strings.Builder, v any) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString("bool:" + strconv.FormatBool(t))
	case string:
		b.WriteString("string:" + smartjson.Quote(t, smartjson.StyleCompact))
	case *smartjson.Array:
		b.WriteByte('[')
		for i, e := range t.Elems() {
			if i > 0 {
				b.WriteByte(',')
			}
			writeTyped(b, e)
		}
		b.WriteByte(']')
	case *smartjson.Object:
		b.WriteByte('{')
		for i, k := range t.Keys() {
			if i > 0 {
				b.WriteByte(',')
			}
			e, _ := t.Get(k)
			b.WriteString(smartjson.Quote(k, smartjson.StyleCompact) + ":")
			writeTyped(b, e)
		}
		b.WriteByte('}')
	case smartjson.KeySet:
		b.WriteString("other:LinkedKeySet")
	default:
		b.WriteString(numberTag(v) + ":" + smartjson.ToString(v))
	}
}

func numberTag(v any) string {
	switch smartjson.ClassName(v) {
	case "java.lang.Integer":
		return "int"
	case "java.lang.Long":
		return "long"
	case "java.math.BigInteger":
		return "bigint"
	case "java.lang.Double":
		return "double"
	case "java.lang.Float":
		return "float"
	case "java.math.BigDecimal":
		return "bigdec"
	}
	return "other"
}
