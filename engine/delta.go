package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rclone/rclone/fs/config"
)

type graphError struct {
	Status  int
	Message string
}

func (e *graphError) Error() string { return fmt.Sprintf("Microsoft (%d): %s", e.Status, e.Message) }

type deltaCache struct {
	Drive    string       `json:"drive"`
	Nodes    map[string]M `json:"nodes"`
	Next     string       `json:"next"`
	Delta    string       `json:"delta"`
	Complete bool         `json:"complete"`
	Updated  string       `json:"updated"`
}

var cacheDir string
var scanInfo = M{"phase": "idle", "files": 0, "pages": 0, "version": 0}
var indexBySize = map[int64][]Item{}
var indexVersion int64
var activeScanCancel context.CancelFunc

func publishIndexLocked(items []Item, complete bool) {
	index = items
	indexComplete = complete
	indexBySize = make(map[int64][]Item)
	for _, i := range items {
		indexBySize[i.Size] = append(indexBySize[i.Size], i)
	}
	indexVersion++
}
func cacheFile(drive string) string {
	sum := sha256.Sum256([]byte(drive))
	return filepath.Join(cacheDir, "cloud-"+hex.EncodeToString(sum[:8])+".json")
}
func loadDelta(drive string) *deltaCache {
	c := &deltaCache{Drive: drive, Nodes: map[string]M{}}
	if cacheDir != "" {
		if raw, e := os.ReadFile(cacheFile(drive)); e == nil {
			var saved deltaCache
			if json.Unmarshal(raw, &saved) == nil && saved.Drive == drive && saved.Nodes != nil && (saved.Next == "" || allowedGraph(saved.Next)) && (saved.Delta == "" || allowedGraph(saved.Delta)) {
				c = &saved
			}
		}
	}
	return c
}
func saveDelta(c *deltaCache) error {
	if cacheDir == "" {
		return nil
	}
	raw, e := json.Marshal(c)
	if e != nil {
		return e
	}
	file := cacheFile(c.Drive)
	if e = os.WriteFile(file+".tmp", raw, 0600); e != nil {
		return e
	}
	return os.Rename(file+".tmp", file)
}
func mergeDelta(old, change M) M {
	if old == nil {
		old = M{}
	}
	// An omitted hash on changed content must not retain an earlier checksum.
	if f, ok := change["file"].(map[string]any); ok {
		copyFile := M{}
		for k, v := range f {
			copyFile[k] = v
		}
		old["file"] = copyFile
	} else if etag := str(change, "eTag"); etag != "" && etag != str(old, "eTag") {
		if f, ok := old["file"].(map[string]any); ok {
			delete(f, "hashes")
		}
	}
	for k, v := range change {
		if k != "file" {
			old[k] = v
		}
	}
	if _, deleted := change["deleted"]; !deleted {
		delete(old, "deleted")
	}
	return old
}
func deltaItems(nodes map[string]M) []Item {
	paths := map[string]string{}
	valid := map[string]bool{}
	visiting := map[string]bool{}
	var resolve func(string) (string, bool)
	resolve = func(id string) (string, bool) {
		if p, known := paths[id]; known {
			return p, valid[id]
		}
		m, exists := nodes[id]
		if !exists || visiting[id] {
			return "", false
		}
		visiting[id] = true
		defer delete(visiting, id)
		if _, gone := m["deleted"]; gone {
			paths[id] = ""
			return "", false
		}
		if _, remote := m["remoteItem"]; remote {
			paths[id] = ""
			return "", false
		}
		special := str(child(m, "specialFolder"), "name")
		if strings.EqualFold(special, "vault") || strings.EqualFold(special, "personalVault") {
			paths[id] = ""
			return "", false
		}
		if _, root := m["root"]; root {
			paths[id] = ""
			valid[id] = true
			return "", true
		}
		parent := str(child(m, "parentReference"), "id")
		base, ok := resolve(parent)
		name := str(m, "name")
		if !ok || name == "" {
			return "", false
		}
		p := name
		if base != "" {
			p = base + "/" + name
		}
		paths[id] = p
		valid[id] = true
		return p, true
	}
	items := make([]Item, 0, len(nodes))
	for id, m := range nodes {
		if _, file := m["file"]; !file {
			continue
		}
		if p, ok := resolve(id); ok {
			items = append(items, toItem(m, p))
		}
	}
	return items
}
func validateDelta(nodes map[string]M) error {
	for id, m := range nodes {
		if _, file := m["file"]; !file {
			continue
		}
		seen := map[string]bool{}
		cursor := id
		for {
			node, exists := nodes[cursor]
			if !exists || seen[cursor] {
				return errors.New("OneDrive has unresolved parent folders. Continue scanning before classifying missing originals")
			}
			seen[cursor] = true
			if _, deleted := node["deleted"]; deleted {
				break
			}
			if _, remote := node["remoteItem"]; remote {
				break
			}
			special := str(child(node, "specialFolder"), "name")
			if strings.EqualFold(special, "vault") || strings.EqualFold(special, "personalVault") {
				break
			}
			if _, root := node["root"]; root {
				break
			}
			if str(node, "name") == "" {
				return errors.New("OneDrive returned an incomplete file record")
			}
			cursor = str(child(node, "parentReference"), "id")
		}
	}
	return nil
}
func syncDelta(ctx context.Context) ([]Item, error) {
	drive := config.GetValue("phone", "drive_id")
	c := loadDelta(drive)
	if c.Next == "" && !c.Complete {
		c.Nodes = map[string]M{}
	}
	resuming := c.Next != ""
	incremental := !resuming && c.Complete && c.Delta != ""
	mu.Lock()
	publishIndexLocked(deltaItems(c.Nodes), false)
	scanInfo = M{"phase": "indexing", "files": len(index), "pages": 0, "cached": len(index), "resuming": resuming, "incremental": incremental, "version": indexVersion}
	mu.Unlock()
	firstURL := rootURL("") + "/delta?$top=500&$select=" + url.QueryEscape("id,name,size,file,folder,root,parentReference,deleted,remoteItem,specialFolder,eTag,lastModifiedDateTime,webUrl,image,video,photo")
	u := c.Next
	if u == "" {
		if incremental {
			u = c.Delta
		} else {
			u = firstURL
		}
	}
	c.Complete = false
	c.Next = u
	lastSave := time.Now()
	pages := 0
	restarted := false
	checkpoint := func() error { return saveDelta(c) }
	fail := func(err error) ([]Item, error) {
		saveErr := checkpoint()
		mu.Lock()
		scanInfo["phase"] = "paused"
		scanInfo["error"] = err.Error()
		scanInfo["saved"] = saveErr == nil
		mu.Unlock()
		if saveErr != nil {
			return nil, fmt.Errorf("%v; checkpoint could not be saved: %v", err, saveErr)
		}
		return nil, err
	}
	for u != "" {
		if canceled.Load() {
			return fail(errors.New("Scan paused; continue comparison to resume saved progress"))
		}
		page, e := graph(ctx, "GET", u, nil, nil)
		if e != nil {
			var ge *graphError
			if errors.As(e, &ge) && ge.Status == 410 && !restarted {
				c = &deltaCache{Drive: drive, Nodes: map[string]M{}, Next: firstURL}
				u = firstURL
				restarted = true
				mu.Lock()
				publishIndexLocked(nil, false)
				scanInfo["phase"] = "rebuilding"
				scanInfo["version"] = indexVersion
				mu.Unlock()
				continue
			}
			return fail(e)
		}
		for _, v := range list(page, "value") {
			m, ok := v.(map[string]any)
			if !ok || str(m, "id") == "" {
				continue
			}
			id := str(m, "id")
			c.Nodes[id] = mergeDelta(c.Nodes[id], m)
		}
		hasRoot := false
		for _, node := range c.Nodes {
			if _, ok := node["root"]; ok {
				hasRoot = true
				break
			}
		}
		if !hasRoot {
			root, e := graph(ctx, "GET", rootURL("")+"?$select=id,name", nil, nil)
			if e != nil {
				return fail(e)
			}
			id := str(root, "id")
			if id == "" {
				return fail(errors.New("OneDrive did not identify the root folder"))
			}
			root["root"] = M{}
			c.Nodes[id] = root
		}
		next, delta := str(page, "@odata.nextLink"), str(page, "@odata.deltaLink")
		if next != "" && !allowedGraph(next) || delta != "" && !allowedGraph(delta) {
			return fail(errors.New("Microsoft returned an unsupported continuation URL"))
		}
		if next == "" && delta == "" {
			return fail(errors.New("Microsoft did not confirm that the cloud index is complete"))
		}
		items := deltaItems(c.Nodes)
		pages++
		done := next == ""
		if done {
			if e := validateDelta(c.Nodes); e != nil {
				return fail(e)
			}
		}
		c.Next = next
		if done {
			c.Delta = delta
			c.Complete = true
			c.Updated = time.Now().Format(time.RFC3339)
		}
		if done || time.Since(lastSave) >= 5*time.Second {
			if e = checkpoint(); e != nil {
				c.Complete = false
				mu.Lock()
				publishIndexLocked(items, false)
				mu.Unlock()
				return fail(fmt.Errorf("Cloud progress could not be saved: %v", e))
			}
			lastSave = time.Now()
		}
		mu.Lock()
		publishIndexLocked(items, done)
		scanInfo["files"] = len(items)
		scanInfo["pages"] = pages
		scanInfo["version"] = indexVersion
		scanInfo["saved"] = true
		if done {
			scanInfo["phase"] = "complete"
			scanInfo["updated"] = c.Updated
		}
		mu.Unlock()
		if done {
			return items, nil
		}
		u = next
	}
	return fail(errors.New("Cloud index did not finish"))
}
