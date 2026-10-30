package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func responseJSON(status int, v any) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(encode(v))), Header: make(http.Header)}
}
func fixture() []Item {
	return []Item{{ID: "a", Name: "renamed.jpg", Path: "one/renamed.jpg", Size: 42, Hash: "same", ETag: "a1"}, {ID: "b", Name: "original.jpg", Path: "two/original.jpg", Size: 42, Hash: "same", ETag: "b1"}}
}
func TestGroupingUsesSizeAndHashNotNames(t *testing.T) {
	files := append(fixture(), Item{ID: "c", Size: 43, Hash: "same"}, Item{ID: "d", Size: 42}, Item{ID: "e", Size: 42})
	groups := groupItems(files)
	if len(groups) != 1 || len(groups[0].Files) != 2 || groups[0].Savings != 42 {
		t.Fatalf("Unexpected groups: %+v", groups)
	}
}
func TestMatchRequiresCompleteIndexAndHandlesMissingHashes(t *testing.T) {
	publishIndexLocked(fixture(), false)
	if got, _ := match(99, ""); got["status"] != "pending" {
		t.Fatal("Incomplete index claimed a missing original")
	}
	if got, _ := match(42, "same"); got["status"] != "uploaded" {
		t.Fatal("An early positive match was hidden")
	}
	publishIndexLocked(fixture(), true)
	for _, tt := range []struct {
		size         int64
		hash, status string
	}{{42, "same", "uploaded"}, {42, "", "hash"}, {42, "different", "missing"}, {99, "", "missing"}} {
		got, err := match(tt.size, tt.hash)
		if err != nil || got["status"] != tt.status {
			t.Fatalf("%+v: %v %v", tt, got, err)
		}
	}
	publishIndexLocked(append(index, Item{ID: "unknown", Size: 42}), true)
	got, _ := match(42, "different")
	if got["status"] != "unknown" {
		t.Fatal("Missing cloud checksum treated as missing upload")
	}
}
func TestQuickXorOriginalFDAndEncoding(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("jRRDVA==")
	f, err := os.CreateTemp(t.TempDir(), "original")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Seek(0, 0)
	got, err := hashFD(duplicateTestFD(t,f))
	f.Close()
	if err != nil || got != "jaDAEKgAAAAAAAAABAAAAAAAAAA=" {
		t.Fatalf("%s %v", got, err)
	}
	if len(hashHex(bytes.Repeat([]byte{0, 255, 12}, 100000))) != 40 {
		t.Fatal("Wrong hash length")
	}
}
func TestDeleteKeepsCopyAndChecksRevisionBeforeNetwork(t *testing.T) {
	groups := groupItems(fixture())
	reports = map[string]report{"folder": {Groups: groups, Revision: "r"}}
	client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("Invalid deletion reached network")
		return nil, errors.New("unexpected")
	})}
	for _, m := range []M{{"mode": "folder", "revision": "obsolete", "ids": []any{"a"}}, {"mode": "folder", "revision": "r", "ids": []any{"a", "b"}}, {"mode": "folder", "revision": "r", "ids": []any{"not-listed"}}} {
		if _, err := deleteCopies(context.Background(), m); err == nil {
			t.Fatal("Accepted unsafe selection")
		}
	}
}
func TestChangedKeeperBlocksDelete(t *testing.T) {
	groups := groupItems(fixture())
	reports = map[string]report{"all": {Groups: groups, Revision: "r"}}
	canceled.Store(false)
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("Changed keeper allowed deletion")
		}
		return responseJSON(200, M{"id": "b", "size": 42, "eTag": "changed", "file": M{"hashes": M{"quickXorHash": "same"}}}), nil
	})}
	result, err := deleteCopies(context.Background(), M{"mode": "all", "revision": "r", "ids": []any{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.(M)["error"] == nil {
		t.Fatal("Changed keeper was accepted")
	}
}
func TestDeleteUsesIfMatchAndNeverRetriesUncertainDelete(t *testing.T) {
	files := fixture()
	groups := groupItems(files)
	reports = map[string]report{"all": {Groups: groups, Revision: "r"}}
	canceled.Store(false)
	calls := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			calls++
			if r.Header.Get("If-Match") != "a1" {
				t.Fatal("Delete lacks expected ETag")
			}
			return nil, errors.New("lost response")
		}
		i := files[0]
		if strings.HasSuffix(r.URL.Path, "/b") {
			i = files[1]
		}
		return responseJSON(200, M{"id": i.ID, "size": i.Size, "eTag": i.ETag, "file": M{"hashes": M{"quickXorHash": i.Hash}}}), nil
	})}
	result, err := deleteCopies(context.Background(), M{"mode": "all", "revision": "r", "ids": []any{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.(M)["error"] == nil || indexComplete {
		t.Fatal("Uncertain deletion was retried or left current index")
	}
	if _, ok := reports["all"]; ok {
		t.Fatal("Obsolete report retained")
	}
}
func TestFailedScanDoesNotPublishPartialIndex(t *testing.T) {
	indexComplete = true
	canceled.Store(false)
	pages := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		if pages == 1 {
			return responseJSON(200, M{"value": []any{M{"id": "x", "name": "file", "file": M{}}}, "@odata.nextLink": graphRoot + "/next"}), nil
		}
		return responseJSON(403, M{"error": M{"message": "denied"}}), nil
	})}
	if _, err := doScan(context.Background(), M{"mode": "all"}); err == nil || indexComplete {
		t.Fatal("Published incomplete cloud scan")
	}
}
func TestTrustBoundariesAndUnicode(t *testing.T) {
	for _, u := range []string{"http://graph.microsoft.com/v1.0/me", "https://graph.microsoft.com.evil.test/v1.0/me", "https://user:pass@graph.microsoft.com/v1.0/me", "https://graph.microsoft.com/v2/me"} {
		if allowedGraph(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"https://a.1drv.com/x", "https://a.sharepoint.com/x", "https://graph.microsoft.com/x", "https://my.microsoftpersonalcontent.com/personal/upload"} {
		if !trustedMedia(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"http://a.1drv.com/x", "https://a.1drv.com.evil.test/x", "https://a.1drv.com:123/x", "https://my.microsoftpersonalcontent.com.evil.test/x"} {
		if trustedMedia(u) {
			t.Fatal(u)
		}
	}
	for _, s := range []string{"..", "bad/name", "bad\\name", "trail.", "file?"} {
		if validSegment(s) {
			t.Fatal(s)
		}
	}
	text := "旅行 📷.jpg"
	var m M
	if err := json.Unmarshal([]byte(encode(M{"name": text})), &m); err != nil || m["name"] != text {
		t.Fatal("Unicode changed")
	}
}

func TestUploadStreamsChunksAndRefusesOverwrite(t *testing.T) {
	data := bytes.Repeat([]byte{11}, 5*1024*1024+17)
	hashBytes, _ := hex.DecodeString(hashHex(data))
	expected := base64.StdEncoding.EncodeToString(hashBytes)
	f, err := os.CreateTemp(t.TempDir(), "original")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Seek(0, 0)
	defer f.Close()
	publishIndexLocked(nil, true)
	canceled.Store(false)
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || !strings.Contains(r.URL.Path, "/root:/photo.jpg:/createUploadSession") {
			t.Fatalf("Unexpected Graph request %s %s", r.Method, r.URL.Path)
		}
		var payload M
		json.NewDecoder(r.Body).Decode(&payload)
		if child(payload, "item")["@microsoft.graph.conflictBehavior"] != "fail" {
			t.Fatal("Upload could overwrite existing cloud file")
		}
		return responseJSON(200, M{"uploadUrl": "https://test.1drv.com/upload"}), nil
	})}
	old := uploadClient
	t.Cleanup(func() { uploadClient = old })
	chunks := 0
	uploadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		chunks++
		if r.Header.Get("Authorization") != "" {
			t.Fatal("OAuth header leaked to upload URL")
		}
		body, _ := io.ReadAll(r.Body)
		if chunks == 1 {
			if len(body) != 5*1024*1024 || r.Header.Get("Content-Range") != "bytes 0-5242879/5242897" {
				t.Fatal("Invalid first chunk")
			}
			return responseJSON(202, M{"nextExpectedRanges": []string{"5242880-"}}), nil
		}
		if len(body) != 17 || r.Header.Get("Content-Range") != "bytes 5242880-5242896/5242897" {
			t.Fatal("Invalid final chunk")
		}
		return responseJSON(201, M{"id": "new", "name": "photo.jpg", "size": len(data), "file": M{"hashes": M{"quickXorHash": expected}}}), nil
	})}
	result, err := upload(context.Background(), M{"fd": float64(duplicateTestFD(t,f)), "size": float64(len(data)), "hash": expected, "name": "photo.jpg", "destination": ""})
	if err != nil {
		t.Fatal(err)
	}
	if chunks != 2 || result.(M)["status"] != "uploaded" {
		t.Fatalf("%+v", result)
	}
}

func TestRcloneOAuthStartsWithoutClientIDSetup(t *testing.T) {
	client = nil
	initialized = false
	if err := Initialize(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := startLogin(); err != nil {
		t.Fatal(err)
	}
	auth := ""
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		mu.Lock()
		auth, _ = login["url"].(string)
		status := login["status"]
		failure := login["error"]
		mu.Unlock()
		if status == "error" {
			t.Fatalf("OAuth did not start: %v", failure)
		}
		if auth != "" {
			break
		}
	}
	if !strings.HasPrefix(auth, "http://127.0.0.1:53682/") && !strings.HasPrefix(auth, "http://localhost:53682/") {
		t.Fatalf("No native Rclone OAuth URL: %q", auth)
	}
	Call("cancel", "{}")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		mu.Lock()
		running := login["status"] == "running"
		mu.Unlock()
		if !running {
			return
		}
	}
	t.Fatal("OAuth did not stop after cancellation")
}
