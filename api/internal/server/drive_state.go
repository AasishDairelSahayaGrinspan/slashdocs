package server

import (
	"encoding/json"
	"errors"
)

// driveOpenState is the payload Google Drive sends in the `state` query
// parameter when a user picks "Open with -> aidocs" on a file:
//
//	{"ids":[fileId],"action":"open","userId":"...","resourceKeys":{fileId: key}}
type driveOpenState struct {
	IDs          []string          `json:"ids"`
	Action       string            `json:"action"`
	UserID       string            `json:"userId"`
	ResourceKeys map[string]string `json:"resourceKeys"`
}

// parseDriveState JSON-unmarshals the `state` value. The caller passes
// c.Query("state"), which Gin has already URL-decoded, so we must NOT decode
// again — a second decode turns '+' into a space and errors on a stray '%'. It
// requires at least one non-empty file id; other fields are optional. The
// `action` field is returned verbatim (the handler checks it == "open").
func parseDriveState(raw string) (driveOpenState, error) {
	if raw == "" {
		return driveOpenState{}, errors.New("empty state")
	}
	var st driveOpenState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return driveOpenState{}, err
	}
	if len(st.IDs) == 0 || st.IDs[0] == "" {
		return driveOpenState{}, errors.New("state missing file id")
	}
	return st, nil
}
