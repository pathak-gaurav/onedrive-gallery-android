package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func deltaRoot() M { return M{"id": "root", "name": "OneDrive", "root": M{}, "folder": M{}} }
func deltaFile(id, parent, name, hash string) M {
	return M{"id": id, "name": name, "parentReference": M{"id": parent}, "size": float64(42), "file": M{"mimeType": "image/jpeg", "hashes": M{"quickXorHash": hash}}, "eTag": "etag"}
}
func setupDelta(t *testing.T) {
	t.Helper()
	previous := cacheDir
	cacheDir = t.TempDir()
	canceled.Store(false)
	t.Cleanup(func() { cacheDir = previous; canceled.Store(false) })
}
func TestLiveMatchAppearsBeforeLastCloudPage(t *testing.T) {
	setupDelta(t)
	waiting := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "page2") {
			close(waiting)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return responseJSON(200, M{"value": []any{}, "@odata.deltaLink": graphRoot + "/changes"}), nil
		}
		return responseJSON(200, M{"value": []any{deltaRoot(), deltaFile("a", "root", "a.jpg", "same")}, "@odata.nextLink": graphRoot + "/page2"}), nil
	})}
	go func() { _, e := doScan(context.Background(), M{"mode": "all"}); result <- e }()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("First page was not published")
	}
	if got, _ := match(42, "same"); got["status"] != "uploaded" {
		t.Fatal("First-page match was delayed")
	}
	if got, _ := match(99, ""); got["status"] != "pending" {
		t.Fatal("Incomplete scan claimed missing")
	}
	close(release)
	if e := <-result; e != nil {
		t.Fatal(e)
	}
	if got, _ := match(99, ""); got["status"] != "missing" {
		t.Fatal("Complete index did not classify unmatched original")
	}
}
func TestInterruptedScanResumesItsSavedNextPage(t *testing.T) {
	setupDelta(t)
	requests := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return responseJSON(200, M{"value": []any{deltaRoot(), deltaFile("a", "root", "a.jpg", "same")}, "@odata.nextLink": graphRoot + "/resume"}), nil
		}
		return responseJSON(403, M{"error": M{"message": "offline"}}), nil
	})}
	if _, e := syncDelta(context.Background()); e == nil {
		t.Fatal("Incomplete scan succeeded")
	}
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.String() != graphRoot+"/resume" {
			t.Fatal("Restarted enumeration instead of resuming")
		}
		return responseJSON(200, M{"value": []any{deltaFile("b", "root", "b.jpg", "other")}, "@odata.deltaLink": graphRoot + "/changes"}), nil
	})}
	items, e := syncDelta(context.Background())
	if e != nil || len(items) != 2 || requests != 3 {
		t.Fatalf("%v %v requests=%d", items, e, requests)
	}
}
func TestDeltaRefreshReadsOnlyChangesAndResolvesRenamedFolder(t *testing.T) {
	setupDelta(t)
	c := &deltaCache{Nodes: map[string]M{"root": deltaRoot(), "folder": {"id": "folder", "name": "Old", "folder": M{}, "parentReference": M{"id": "root"}}, "a": deltaFile("a", "root", "gone.jpg", "same")}, Complete: true, Delta: graphRoot + "/changes"}
	if e := saveDelta(c); e != nil {
		t.Fatal(e)
	}
	requests := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.String() != graphRoot+"/changes" {
			t.Fatal("Did not use the saved change feed")
		}
		return responseJSON(200, M{"value": []any{M{"id": "folder", "name": "Renamed"}, M{"id": "a", "deleted": M{}}, deltaFile("b", "folder", "new.jpg", "other")}, "@odata.deltaLink": graphRoot + "/changes2"}), nil
	})}
	items, e := syncDelta(context.Background())
	if e != nil || requests != 1 || len(items) != 1 || items[0].Path != "Renamed/new.jpg" {
		t.Fatalf("%+v %v requests=%d", items, e, requests)
	}
	if got, _ := match(42, "same"); got["status"] != "missing" {
		t.Fatal("Deleted cloud original still matched")
	}
}
func TestExpiredDeltaTokenRebuildsWithoutOldMatches(t *testing.T) {
	setupDelta(t)
	saveDelta(&deltaCache{Nodes: map[string]M{"root": deltaRoot(), "a": deltaFile("a", "root", "old.jpg", "old")}, Complete: true, Delta: graphRoot + "/expired"})
	requests := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if strings.Contains(r.URL.Path, "expired") {
			return responseJSON(410, M{"error": M{"message": "expired"}}), nil
		}
		return responseJSON(200, M{"value": []any{deltaRoot(), deltaFile("b", "root", "new.jpg", "new")}, "@odata.deltaLink": graphRoot + "/fresh"}), nil
	})}
	items, e := syncDelta(context.Background())
	if e != nil || requests != 2 || len(items) != 1 || items[0].ID != "b" {
		t.Fatalf("%+v %v", items, e)
	}
}
func TestChangedContentCannotKeepAnOmittedOldChecksum(t *testing.T) {
	node := deltaFile("a", "root", "photo.jpg", "old")
	node = mergeDelta(node, M{"id": "a", "eTag": "new", "file": M{"mimeType": "image/jpeg"}})
	if toItem(node, "").Hash != "" {
		t.Fatal("Old hash survived a changed file")
	}
}
func TestPhoneDeletionRequiresFreshCloudOriginal(t *testing.T) {
	publishIndexLocked(fixture(), false)
	if !strings.Contains(Call("verifyMatch", `{"size":99,"hash":"unseen"}`), "error") {
		t.Fatal("Unmatched original allowed phone deletion during a partial scan")
	}
	client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return responseJSON(200, M{"id": "a", "size": 42, "file": M{"hashes": M{"quickXorHash": "same"}}}), nil
	})}
	if !strings.Contains(Call("verifyMatch", `{"size":42,"hash":"same"}`), `"verified":true`) {
		t.Fatal("Fresh positive match could not be verified before scan completion")
	}
	publishIndexLocked(fixture(), true)
	client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return responseJSON(200, M{"id": "a", "size": 42, "file": M{"hashes": M{"quickXorHash": "changed"}}}), nil
	})}
	if !strings.Contains(Call("verifyMatch", `{"size":42,"hash":"same"}`), "error") {
		t.Fatal("A stale cloud match allowed phone deletion")
	}
	client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if !strings.Contains(Call("verifyMatch", `{"size":42,"hash":"same"}`), "error") {
		t.Fatal("Network failure allowed phone deletion")
	}
}

func BenchmarkLargeLibraryMatch(b *testing.B) {
	items := make([]Item, 100000)
	for i := range items {
		items[i] = Item{ID: fmt.Sprint(i), Size: int64(i + 1), Hash: "hash"}
	}
	publishIndexLocked(items, true)
	b.Run("indexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			got, _ := match(100000, "hash")
			if got["status"] != "uploaded" {
				b.Fatal("missing match")
			}
		}
	})
	b.Run("previous_linear_lookup", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			found := false
			for _, item := range index {
				if item.Size == 100000 && item.Hash == "hash" {
					found = true
				}
			}
			if !found {
				b.Fatal("missing match")
			}
		}
	})
}
