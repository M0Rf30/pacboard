// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"encoding/json"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Str reads a string at path, "" when absent or not a string.
func Str(obj map[string]any, path ...string) string {
	v, _, _ := unstructured.NestedFieldNoCopy(obj, path...)
	s, _ := v.(string)

	return s
}

// Slice reads a list of objects at path; non-object items are dropped.
func Slice(obj map[string]any, path ...string) []map[string]any {
	v, _, _ := unstructured.NestedFieldNoCopy(obj, path...)
	list, _ := v.([]any)

	out := make([]map[string]any, 0, len(list))

	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}

	return out
}

// Time reads an RFC 3339 timestamp at path, nil when absent or malformed.
func Time(obj map[string]any, path ...string) *time.Time {
	s := Str(obj, path...)
	if s == "" {
		return nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}

	return &t
}

// Stringify renders a parameter value: strings as is, lists and objects as compact JSON.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}

		return string(b)
	}
}

// Params maps spec.params of a PipelineRun to stringified name/value pairs.
func Params(plr map[string]any) []Param {
	out := []Param{}

	for _, p := range Slice(plr, "spec", "params") {
		out = append(out, Param{Name: Str(p, "name"), Value: Stringify(p["value"])})
	}

	return out
}
