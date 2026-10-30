package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rclone/rclone/backend/onedrive"
	"github.com/rclone/rclone/backend/onedrive/quickxorhash"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/rc"
	"github.com/rclone/rclone/lib/oauthutil"
)

const graphRoot = "https://graph.microsoft.com/v1.0"

type M = map[string]any
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Path     string `json:"path"`
	Modified string `json:"modified"`
	Taken    string `json:"taken"`
	Hash     string `json:"hash"`
	SHA1     string `json:"sha1"`
	ETag     string `json:"etag"`
	WebURL   string `json:"webUrl"`
	Type     string `json:"type"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration int64  `json:"duration"`
}
type Group struct {
	ID       string `json:"id"`
	Files    []Item `json:"files"`
	Size     int64  `json:"size"`
	Savings  int64  `json:"savings"`
	SameSize bool   `json:"sameSize"`
}
type report struct {
	Groups     []Group `json:"groups"`
	Revision   string  `json:"revision"`
	ScannedAt  string  `json:"scannedAt"`
	FolderID   string  `json:"folderId"`
	FolderPath string  `json:"folderPath"`
}

var mu sync.Mutex
var client *http.Client
var login = M{"status": "idle"}
var reports = map[string]report{}
var index []Item
var indexComplete bool
var canceled atomic.Bool
var initialized bool
var uploadClient = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Unexpected upload redirect") }}

func encode(v any) string      { b, _ := json.Marshal(v); return string(b) }
func str(m M, k string) string { v, _ := m[k].(string); return v }
func num(m M, k string) int64  { v, _ := m[k].(float64); return int64(v) }
func child(m M, k string) M {
	v, _ := m[k].(map[string]any)
	if v == nil {
		return M{}
	}
	return v
}
func list(m M, k string) []any { v, _ := m[k].([]any); return v }
func Initialize(dir, certFile string) error {
	if initialized {
		return nil
	}
	initialized = true
	cacheDir = dir
	log.SetOutput(io.Discard)
	if certFile != "" {
		os.Setenv("SSL_CERT_FILE", certFile)
	}
	configfile.Install()
	if err := config.SetConfigPath(path.Join(dir, "rclone.conf")); err != nil {
		return err
	}
	oauthutil.OpenURL = func(u string) error { mu.Lock(); login["url"] = u; mu.Unlock(); return nil }
	return nil
}
func getClient() (*http.Client, error) {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return client, nil
	}
	if config.GetValue("phone", "token") == "" {
		return nil, errors.New("Sign in to OneDrive first")
	}
	ri, err := fs.Find("onedrive")
	if err != nil {
		return nil, err
	}
	mapper := fs.ConfigMap(ri.Prefix, ri.Options, "phone", nil)
	out, err := onedrive.Config(context.Background(), "phone", mapper, fs.ConfigIn{})
	if err != nil {
		return nil, err
	}
	opts, ok := out.OAuth.(*oauthutil.Options)
	if !ok {
		return nil, errors.New("Rclone OAuth configuration unavailable")
	}
	c, _, err := oauthutil.NewClient(context.Background(), "phone", mapper, opts.OAuth2Config)
	if err != nil {
		return nil, err
	}
	c.Timeout = 2 * time.Minute
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return errors.New("Unexpected Microsoft API redirect")
	}
	client = c
	return c, nil
}
func startLogin() (any, error) {
	mu.Lock()
	if login["status"] == "running" {
		mu.Unlock()
		return M{"status": "running"}, nil
	}
	login = M{"status": "running"}
	client = nil
	mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		out, err := config.CreateRemote(ctx, "phone", "onedrive", rc.Params{"config_is_local": "true", "config_type": "onedrive", "disable_site_permission": "true"}, config.UpdateRemoteOpt{NonInteractive: true})
		for steps := 0; err == nil && out != nil && out.State != "" && steps < 12; steps++ {
			if out.Error != "" {
				err = errors.New(out.Error)
				break
			}
			if out.Option == nil {
				err = errors.New("Unexpected Rclone sign-in state")
				break
			}
			answer := ""
			switch out.Option.Name {
			case "config_driveid":
				var d M
				d, err = graph(ctx, "GET", graphRoot+"/me/drive", nil, nil)
				answer = str(d, "id")
				if err == nil && answer == "" {
					err = errors.New("Microsoft did not return your OneDrive")
				}
			case "config_drive_ok":
				answer = "true"
			default:
				err = fmt.Errorf("Microsoft sign-in needs an unsupported choice: %s", out.Option.Name)
			}
			if err != nil {
				break
			}
			out, err = config.UpdateRemote(ctx, "phone", rc.Params{}, config.UpdateRemoteOpt{Continue: true, State: out.State, Result: answer})
		}
		if err == nil && (config.GetValue("phone", "drive_id") == "" || out != nil && out.State != "") {
			err = errors.New("OneDrive configuration did not finish")
		}
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			login = M{"status": "error", "error": err.Error()}
		} else {
			login = M{"status": "complete"}
			publishIndexLocked(nil, false)
			reports = map[string]report{}
		}
	}()
	return M{"status": "running"}, nil
}
func allowedGraph(u string) bool {
	p, e := url.Parse(u)
	return e == nil && p.Scheme == "https" && p.Host == "graph.microsoft.com" && p.User == nil && strings.HasPrefix(p.Path, "/v1.0/")
}
func graph(ctx context.Context, method, u string, body any, headers M) (M, error) {
	if !allowedGraph(u) {
		return nil, errors.New("Unsupported Microsoft API URL")
	}
	c, err := getClient()
	if err != nil {
		return nil, err
	}
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprint(v))
		}
		resp, err := c.Do(req)
		if err != nil {
			return nil, errors.New("Microsoft could not be reached. Check your connection and refresh before retrying")
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if (method == "GET") && (resp.StatusCode == 429 || resp.StatusCode == 503) && attempt < 3 {
			s, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if s < 1 {
				s = 2 << attempt
			}
			if s > 60 {
				return nil, errors.New("Microsoft asked to slow down. Try again later")
			}
			select {
			case <-time.After(time.Duration(s) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var e M
			json.Unmarshal(raw, &e)
			message := str(child(e, "error"), "message")
			if message == "" {
				message = http.StatusText(resp.StatusCode)
			}
			return nil, &graphError{Status: resp.StatusCode, Message: message}
		}
		m := M{}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &m); err != nil {
				return nil, err
			}
		}
		return m, nil
	}
	return nil, errors.New("Microsoft is busy. Try again later")
}
func itemURL(id string) string {
	return graphRoot + "/drives/" + url.PathEscape(config.GetValue("phone", "drive_id")) + "/items/" + url.PathEscape(id)
}
func rootURL(id string) string {
	if id == "" {
		return graphRoot + "/drives/" + url.PathEscape(config.GetValue("phone", "drive_id")) + "/root"
	}
	return itemURL(id)
}
func toItem(m M, p string) Item {
	f := child(m, "file")
	h := child(f, "hashes")
	image := child(m, "image")
	video := child(m, "video")
	typ := "other"
	mime := str(f, "mimeType")
	if strings.HasPrefix(mime, "image/") {
		typ = "image"
	} else if strings.HasPrefix(mime, "video/") {
		typ = "video"
	} else if strings.Contains(mime, "pdf") || strings.Contains(mime, "document") || strings.HasPrefix(mime, "text/") {
		typ = "document"
	}
	w, hh := num(image, "width"), num(image, "height")
	if w == 0 {
		w, hh = num(video, "width"), num(video, "height")
	}
	return Item{ID: str(m, "id"), Name: str(m, "name"), Size: num(m, "size"), Path: p, Modified: str(m, "lastModifiedDateTime"), Taken: str(child(m, "photo"), "takenDateTime"), Hash: str(h, "quickXorHash"), SHA1: strings.ToLower(str(h, "sha1Hash")), ETag: str(m, "eTag"), WebURL: str(m, "webUrl"), Type: typ, Width: int(w), Height: int(hh), Duration: num(video, "duration")}
}
func children(ctx context.Context, id string) ([]M, error) {
	u := rootURL(id) + "/children?$top=200"
	all := []M{}
	for u != "" {
		if canceled.Load() {
			return nil, errors.New("Stopped")
		}
		m, e := graph(ctx, "GET", u, nil, nil)
		if e != nil {
			return nil, e
		}
		for _, v := range list(m, "value") {
			if x, ok := v.(map[string]any); ok {
				all = append(all, x)
			}
		}
		u = str(m, "@odata.nextLink")
	}
	return all, nil
}
func scan(ctx context.Context, id, p string) ([]Item, error) {
	items := []Item{}
	var visit func(string, string) error
	visit = func(id, p string) error {
		entries, e := children(ctx, id)
		if e != nil {
			return e
		}
		for _, m := range entries {
			if canceled.Load() {
				return errors.New("Stopped")
			}
			name := str(m, "name")
			if len(child(m, "deleted")) > 0 {
				continue
			}
			if _, remote := m["remoteItem"]; remote {
				continue
			}
			special := str(child(m, "specialFolder"), "name")
			if strings.EqualFold(special, "vault") || strings.EqualFold(special, "personalVault") {
				continue
			}
			pp := path.Join(p, name)
			if _, folder := m["folder"]; folder {
				if e = visit(str(m, "id"), pp); e != nil {
					return e
				}
			} else if _, file := m["file"]; file {
				items = append(items, toItem(m, pp))
			}
		}
		return nil
	}
	if err := visit(id, p); err != nil {
		return nil, err
	}
	return items, nil
}
func hashKey(i Item) string {
	if i.Hash != "" {
		return fmt.Sprintf("qx:%d:%s", i.Size, i.Hash)
	}
	if i.SHA1 != "" {
		return fmt.Sprintf("sha1:%d:%s", i.Size, i.SHA1)
	}
	return ""
}
func groupItems(items []Item) []Group {
	by := map[string][]Item{}
	for _, i := range items {
		if k := hashKey(i); k != "" {
			by[k] = append(by[k], i)
		}
	}
	groups := []Group{}
	for k, files := range by {
		if len(files) < 2 {
			continue
		}
		slices.SortFunc(files, func(a, b Item) int { return strings.Compare(a.Path, b.Path) })
		groups = append(groups, Group{ID: base64.RawURLEncoding.EncodeToString([]byte(k)), Files: files, Size: files[0].Size, Savings: files[0].Size * int64(len(files)-1), SameSize: true})
	}
	slices.SortFunc(groups, func(a, b Group) int {
		if a.Savings > b.Savings {
			return -1
		}
		if a.Savings < b.Savings {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return groups
}
func doScan(ctx context.Context, m M) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	mu.Lock()
	activeScanCancel = cancel
	mu.Unlock()
	defer func() { cancel(); mu.Lock(); activeScanCancel = nil; mu.Unlock() }()
	mode := str(m, "mode")
	if mode != "folder" {
		mode = "all"
	}
	id, p := str(m, "folderId"), str(m, "folderPath")
	if mode == "all" {
		id, p = "", ""
		mu.Lock()
		indexComplete = false
		mu.Unlock()
	}
	var items []Item
	var err error
	if mode == "all" {
		items, err = syncDelta(ctx)
	} else {
		items, err = scan(ctx, id, p)
	}
	if err != nil {
		return nil, err
	}
	r := report{Groups: groupItems(items), Revision: strconv.FormatInt(time.Now().UnixNano(), 10), ScannedAt: time.Now().Format(time.RFC3339), FolderID: id, FolderPath: p}
	mu.Lock()
	reports[mode] = r
	if mode == "all" {
		publishIndexLocked(items, true)
	}
	mu.Unlock()
	return M{"report": r, "fileCount": len(items), "mode": mode}, nil
}
func match(size int64, hash string) (M, error) {
	mu.Lock()
	defer mu.Unlock()
	candidates := []Item{}
	unknown := false
	for _, i := range indexBySize[size] {
		if hash != "" && i.Hash == hash {
			candidates = append(candidates, i)
		}
		if i.Hash == "" {
			unknown = true
		}
	}
	if len(candidates) > 0 {
		return M{"status": "uploaded", "matches": candidates}, nil
	}
	if hash == "" {
		for range indexBySize[size] {
			return M{"status": "hash"}, nil
		}
	}
	if !indexComplete {
		return M{"status": "pending"}, nil
	}
	if unknown {
		return M{"status": "unknown", "message": "A same-size OneDrive file has no comparable checksum"}, nil
	}
	return M{"status": "missing"}, nil
}
func hashFD(fd int64) (string, error) {
	f := os.NewFile(uintptr(fd), "phone-original")
	if f == nil {
		return "", errors.New("Cannot open phone file")
	}
	defer f.Close()
	h := quickxorhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}
func same(a, b Item) bool {
	return a.ID == b.ID && a.Size == b.Size && hashKey(a) != "" && hashKey(a) == hashKey(b) && a.ETag == b.ETag && a.ETag != ""
}
func deleteCopies(ctx context.Context, m M) (any, error) {
	mode := str(m, "mode")
	mu.Lock()
	r, ok := reports[mode]
	mu.Unlock()
	if !ok || r.Revision != str(m, "revision") {
		return nil, errors.New("The scan changed. Rescan before deleting")
	}
	selected := map[string]bool{}
	for _, v := range list(m, "ids") {
		s, _ := v.(string)
		selected[s] = true
	}
	if len(selected) == 0 {
		return nil, errors.New("Select copies first")
	}
	plans := [][]Item{}
	known := 0
	for _, g := range r.Groups {
		remove := []Item{}
		keepers := []Item{}
		for _, i := range g.Files {
			if selected[i.ID] {
				remove = append(remove, i)
				known++
			} else {
				keepers = append(keepers, i)
			}
		}
		if len(remove) == 0 {
			continue
		}
		if len(keepers) == 0 {
			return nil, errors.New("Keep at least one copy in every group")
		}
		plans = append(plans, append([]Item{keepers[0]}, remove...))
	}
	if known != len(selected) {
		return nil, errors.New("Selection is obsolete. Rescan before deleting")
	}
	deleted := []string{}
	var failure error
	for _, plan := range plans {
		for _, i := range plan[1:] {
			if canceled.Load() {
				failure = errors.New("Stopped")
				break
			}
			keeper, e := graph(ctx, "GET", itemURL(plan[0].ID), nil, nil)
			if e != nil {
				failure = e
				break
			}
			if !same(plan[0], toItem(keeper, plan[0].Path)) {
				failure = errors.New("A kept copy changed. Rescan before deleting")
				break
			}
			fresh, e := graph(ctx, "GET", itemURL(i.ID), nil, nil)
			if e != nil {
				failure = e
				break
			}
			if !same(i, toItem(fresh, i.Path)) {
				failure = errors.New("A selected copy changed. Rescan before deleting")
				break
			}
			_, e = graph(ctx, "DELETE", itemURL(i.ID), nil, M{"If-Match": i.ETag})
			if e != nil {
				failure = e
				break
			}
			deleted = append(deleted, i.ID)
		}
		if failure != nil {
			break
		}
	}
	mu.Lock()
	indexComplete = false
	delete(reports, mode)
	mu.Unlock()
	result := M{"deleted": deleted}
	if failure != nil {
		result["error"] = failure.Error() + ". Refresh before retrying; a lost response may hide a completed deletion."
	}
	return result, nil
}
func trustedMedia(u string) bool {
	p, e := url.Parse(u)
	if e != nil || p.Scheme != "https" || p.User != nil || p.Port() != "" && p.Port() != "443" {
		return false
	}
	for _, d := range []string{"microsoft.com", "microsoftpersonalcontent.com", "1drv.com", "svc.ms", "live.com", "onedrive.com", "sharepoint.com", "sharepointonline.com"} {
		if p.Hostname() == d || strings.HasSuffix(p.Hostname(), "."+d) {
			return true
		}
	}
	return false
}
func thumbnail(ctx context.Context, id string) (any, error) {
	m, e := graph(ctx, "GET", itemURL(id)+"/thumbnails", nil, nil)
	if e != nil {
		return nil, e
	}
	vals := list(m, "value")
	if len(vals) == 0 {
		return nil, errors.New("No thumbnail available")
	}
	first, _ := vals[0].(map[string]any)
	u := str(child(first, "medium"), "url")
	if !trustedMedia(u) {
		return nil, errors.New("Unsupported thumbnail location")
	}
	c := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 6 || !trustedMedia(r.URL.String()) {
			return errors.New("Unsupported thumbnail redirect")
		}
		return nil
	}}
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, e := c.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("Thumbnail unavailable")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return nil, e
	}
	return M{"data": base64.StdEncoding.EncodeToString(b), "mime": "image/jpeg"}, nil
}
func ensureFolder(ctx context.Context, parent, name string) (string, error) {
	entries, e := children(ctx, parent)
	if e != nil {
		return "", e
	}
	for _, m := range entries {
		if strings.EqualFold(str(m, "name"), name) {
			if _, ok := m["folder"]; !ok {
				return "", errors.New("An upload folder name is already used by a file")
			}
			return str(m, "id"), nil
		}
	}
	m, e := graph(ctx, "POST", rootURL(parent)+"/children", M{"name": name, "folder": M{}, "@microsoft.graph.conflictBehavior": "fail"}, nil)
	return str(m, "id"), e
}
func validSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\:*?\"<>|\x00\r\n") && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, " ")
}
func upload(ctx context.Context, m M) (any, error) {
	f := os.NewFile(uintptr(num(m, "fd")), "phone-original")
	if f == nil {
		return nil, errors.New("Phone file unavailable")
	}
	defer f.Close()
	size := num(m, "size")
	expected := str(m, "hash")
	if size <= 0 {
		return nil, errors.New("The original file size is unavailable")
	}
	h := quickxorhash.New()
	n, e := io.Copy(h, f)
	if e != nil {
		return nil, e
	}
	actualHash := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if n != size || expected != "" && actualHash != expected {
		return nil, errors.New("The phone file changed. Compare again")
	}
	expected = actualHash
	if _, e = f.Seek(0, 0); e != nil {
		return nil, errors.New("This local file cannot be streamed safely")
	}
	matched, e := match(size, expected)
	if e != nil {
		return nil, e
	}
	if matched["status"] == "uploaded" {
		return M{"status": "skipped", "message": "An identical original is already in OneDrive"}, nil
	}
	if matched["status"] != "missing" {
		return nil, errors.New("A same-size OneDrive file needs review before upload")
	}
	dest := strings.Trim(str(m, "destination"), "/")
	parts := strings.Split(dest, "/")
	if dest == "" {
		parts = nil
	}
	name := str(m, "name")
	if !validSegment(name) {
		return nil, errors.New("The filename is not supported by OneDrive")
	}
	for _, s := range parts {
		if !validSegment(s) {
			return nil, errors.New("Use normal folder names separated by / ")
		}
	}
	parent := ""
	for _, s := range parts {
		parent, e = ensureFolder(ctx, parent, s)
		if e != nil {
			return nil, e
		}
	}
	behavior := "fail"
	if str(m, "conflictBehavior") == "rename" {
		behavior = "rename"
	}
	session, e := graph(ctx, "POST", rootURL(parent)+":/"+url.PathEscape(name)+":/createUploadSession", M{"item": M{"@microsoft.graph.conflictBehavior": behavior, "name": name}}, nil)
	if e != nil {
		return nil, e
	}
	uploadURL := str(session, "uploadUrl")
	if !trustedMedia(uploadURL) {
		if parsed, err := url.Parse(uploadURL); err == nil && parsed.Hostname() != "" {
			return nil, fmt.Errorf("Unsupported Microsoft upload host: %s", parsed.Hostname())
		}
		return nil, errors.New("Unsupported Microsoft upload location")
	}
	transport := uploadClient
	buf := make([]byte, 5*1024*1024)
	var result M
	for offset := int64(0); offset < size; {
		if canceled.Load() {
			req, _ := http.NewRequest("DELETE", uploadURL, nil)
			if resp, e := transport.Do(req); e == nil {
				resp.Body.Close()
			}
			return nil, errors.New("Upload stopped")
		}
		amount := min(int64(len(buf)), size-offset)
		if _, e = io.ReadFull(f, buf[:amount]); e != nil {
			return nil, e
		}
		req, _ := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(buf[:amount]))
		req.ContentLength = amount
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+amount-1, size))
		resp, e := transport.Do(req)
		if e != nil {
			return nil, errors.New("Upload connection lost. Compare again before retrying")
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != 202 && resp.StatusCode != 200 && resp.StatusCode != 201 {
			return nil, fmt.Errorf("Upload stopped by Microsoft (%d). Compare again before retrying", resp.StatusCode)
		}
		result = M{}
		if e = json.Unmarshal(raw, &result); e != nil {
			return nil, e
		}
		offset += amount
	}
	if str(result, "id") == "" {
		return nil, errors.New("Microsoft did not confirm the completed upload. Compare again")
	}
	uploadedName := str(result, "name")
	if uploadedName == "" { uploadedName = name }
	uploaded := toItem(result, path.Join(dest, uploadedName))
	mu.Lock()
	publishIndexLocked(append(index, uploaded), indexComplete)
	mu.Unlock()
	if uploaded.Size != size || uploaded.Hash != expected {
		return M{"status": "review", "message": "Upload completed, but Microsoft has not confirmed the original checksum. Compare again", "item": uploaded}, nil
	}
	return M{"status": "uploaded", "item": uploaded}, nil
}
func Call(operation, body string) (result string) {
	defer func() {
		if recover() != nil {
			result = encode(M{"error": "The operation could not finish. Refresh before retrying"})
		}
	}()
	m := M{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return encode(M{"error": "Invalid app request"})
	}
	ctx := context.Background()
	var value any
	var err error
	switch operation {
	case "status":
		mu.Lock()
		value = M{"signedIn": config.GetValue("phone", "drive_id") != "" && config.GetValue("phone", "token") != "", "login": login, "indexComplete": indexComplete, "scan": scanInfo, "indexVersion": indexVersion}
		result = encode(value)
		mu.Unlock()
		return
	case "login":
		value, err = startLogin()
	case "cancel":
		canceled.Store(true)
		mu.Lock()
		activeLogin := login["status"] == "running"
		if activeScanCancel != nil {
			activeScanCancel()
		}
		mu.Unlock()
		if activeLogin {
			if call := rc.Calls.Get("config/oauthstop"); call != nil {
				_, _ = call.Fn(ctx, rc.Params{})
			}
		}
		value = M{"ok": true}
	case "resetCancel":
		canceled.Store(false)
		value = M{"ok": true}
	case "invalidateIndex":
		mu.Lock()
		indexComplete = false
		mu.Unlock()
		value = M{"ok": true}
	case "folders":
		var entries []M
		entries, err = children(ctx, str(m, "id"))
		folders := []M{}
		for _, e := range entries {
			if _, ok := e["folder"]; ok {
				folders = append(folders, M{"id": e["id"], "name": e["name"]})
			}
		}
		value = M{"folders": folders}
	case "scan":
		value, err = doScan(ctx, m)
	case "match":
		value, err = match(num(m, "size"), str(m, "hash"))
	case "matchBatch":
		matches := []M{}
		for _, v := range list(m, "files") {
			f, ok := v.(map[string]any)
			if !ok {
				err = errors.New("Invalid comparison batch")
				break
			}
			one, e := match(num(f, "size"), str(f, "hash"))
			if e != nil {
				err = e
				break
			}
			matches = append(matches, one)
		}
		value = M{"matches": matches}
	case "verifyMatch":
		matched, e := match(num(m, "size"), str(m, "hash"))
		if e != nil {
			err = e
			break
		}
		valid := false
		if files, ok := matched["matches"].([]Item); ok {
			for _, file := range files {
				fresh, e := graph(ctx, "GET", itemURL(file.ID), nil, nil)
				if e != nil {
					continue
				}
				i := toItem(fresh, file.Path)
				if i.Size == num(m, "size") && i.Hash != "" && i.Hash == str(m, "hash") {
					valid = true
					break
				}
			}
		}
		if !valid {
			err = errors.New("An identical OneDrive original could not be verified. Keep the phone file and compare again")
		} else {
			value = M{"verified": true}
		}
	case "preview":
		var metadata M
		metadata, err = graph(ctx, "GET", itemURL(str(m, "id")), nil, nil)
		if err == nil {
			i := toItem(metadata, "")
			u := str(metadata, "@microsoft.graph.downloadUrl")
			if i.Type == "video" {
				if !trustedMedia(u) {
					err = errors.New("OneDrive did not provide a supported streaming preview")
					break
				}
			}
			value = M{"type": i.Type, "name": i.Name, "url": u, "webUrl": i.WebURL}
		}
	case "hash":
		var h string
		h, err = hashFD(num(m, "fd"))
		value = M{"hash": h}
	case "delete":
		value, err = deleteCopies(ctx, m)
	case "thumbnail":
		value, err = thumbnail(ctx, str(m, "id"))
	case "metadata":
		value, err = graph(ctx, "GET", itemURL(str(m, "id")), nil, nil)
	case "upload":
		value, err = upload(ctx, m)
	default:
		err = errors.New("Unknown app operation")
	}
	if err != nil {
		return encode(M{"error": err.Error()})
	}
	return encode(value)
}
func main() {}

// Keep an independent encoding check in tests; OneDrive uses base64, rclone's CLI often displays hex.
func hashHex(b []byte) string {
	h := quickxorhash.New()
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
