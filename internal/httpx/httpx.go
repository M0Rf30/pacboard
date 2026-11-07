// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package httpx holds the JSON response helpers shared by the HTTP layers.
package httpx

import (
	"encoding/json"
	"net/http"
)

// JSON writes v as the JSON body with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status line is out already; an encoding failure can only be a broken connection.
	_ = json.NewEncoder(w).Encode(v) //nolint:errchkjson // v is always one of our response types
}

// Error writes the contract's error body: {"error": "<message>"}.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}
