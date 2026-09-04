package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestVersionEndpoint(t *testing.T) {
	oldVersion, oldRevision := version, revision
	version, revision = "test-version", "deadbeef"
	defer func() { version, revision = oldVersion, oldRevision }()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/version", nil)
	handleVersion(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	var got VersionInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != "test-version" || got.Revision != "deadbeef" {
		t.Fatalf("version info = %#v", got)
	}
}
