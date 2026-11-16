package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// ValueIO reads and writes live kernel values (proc/sys files). It is a hook
// so tests can simulate a kernel that rejects or rewrites a value.
type ValueIO interface {
	Read(path string) (string, error)
	Write(path, value string) error
}

type osIO struct{}

func (osIO) Read(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}

func (osIO) Write(p, v string) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0) // never create live files
	if err != nil {
		return err
	}
	if _, err := f.WriteString(v); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// OSIO is the real filesystem implementation of ValueIO.
var OSIO ValueIO = osIO{}

// Options controls Apply.
type Options struct {
	DryRun  bool
	Now     func() time.Time // default time.Now
	IO      ValueIO          // default OSIO
	Version string           // kerneltuner version recorded in the snapshot
}

// LiveValue is a prior live value recorded in a snapshot.
type LiveValue struct {
	Path  string `json:"path"`
	Prior string `json:"prior"` // raw content as read
}

// FileState is the prior state of a file kerneltuner touches.
type FileState struct {
	Path        string `json:"path"`
	DidNotExist bool   `json:"did_not_exist"`
	Content     []byte `json:"content,omitempty"` // base64 in JSON
	Mode        uint32 `json:"mode,omitempty"`
}

// Snapshot is the rollback record written before anything changes.
type Snapshot struct {
	ID          string      `json:"id"`
	Created     time.Time   `json:"created"`
	Version     string      `json:"kerneltuner_version"`
	Live        []LiveValue `json:"live"`
	Files       []FileState `json:"files"`
	CreatedDirs []string    `json:"created_dirs,omitempty"`
}

// Result describes an Apply run.
type Result struct {
	ID       string   `json:"id,omitempty"`
	Snapshot string   `json:"snapshot,omitempty"` // path of the snapshot file
	DryRun   bool     `json:"dry_run"`
	Ops      []Op     `json:"ops"`
	Hints    []string `json:"hints,omitempty"`
}

var idRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_.-]*$`)

func snapDir(root facts.Root) string {
	return filepath.Join(root.Var, "lib", "kerneltuner", "snapshots")
}

// Apply executes plan: snapshot, managed files, live values, verify. Any
// failure triggers an automatic rollback of this snapshot.
func Apply(plan *Plan, opts Options) (*Result, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.IO == nil {
		opts.IO = OSIO
	}
	res := &Result{DryRun: opts.DryRun, Ops: plan.Ops}
	units := map[string]bool{}
	for _, op := range plan.Ops {
		if op.Unit != "" && !units[op.Unit] {
			units[op.Unit] = true
			res.Hints = append(res.Hints, fmt.Sprintf("limits take effect after: systemctl daemon-reload && systemctl restart %s", op.Unit))
		}
	}
	if opts.DryRun || len(plan.Ops) == 0 {
		return res, nil
	}
	root := plan.Root

	// Group file edits (deterministic order).
	edits := map[string][]Op{}
	for _, op := range plan.Ops {
		if op.File != "" {
			edits[op.File] = append(edits[op.File], op)
		}
	}
	paths := make([]string, 0, len(edits))
	for p := range edits {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Read prior state; nothing is written yet.
	now := opts.Now().UTC()
	snap := &Snapshot{Created: now, Version: opts.Version}
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		prior, err := opts.IO.Read(op.LivePath)
		if err != nil {
			return nil, fmt.Errorf("read live value %s: %w", op.LivePath, err)
		}
		snap.Live = append(snap.Live, LiveValue{Path: op.LivePath, Prior: prior})
	}
	newContent := map[string][]byte{}
	for _, p := range paths {
		fsState := FileState{Path: p}
		prior, err := os.ReadFile(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fsState.DidNotExist = true
			snap.CreatedDirs = append(snap.CreatedDirs, missingDirs(filepath.Dir(p))...)
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", p, err)
		default:
			fsState.Content = prior
			fsState.Mode = 0o644
			if st, serr := os.Stat(p); serr == nil {
				fsState.Mode = uint32(st.Mode().Perm())
			}
		}
		newContent[p] = merge(edits[p][0].FileKind, string(prior), fsState.DidNotExist, edits[p])
		snap.Files = append(snap.Files, fsState)
	}
	snap.CreatedDirs = dedupe(snap.CreatedDirs)

	// 1. Snapshot first.
	dir := snapDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}
	id := now.Format("20060102T150405Z")
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, id+".json")); errors.Is(err, fs.ErrNotExist) {
			break
		}
		id = fmt.Sprintf("%s-%d", now.Format("20060102T150405Z"), n)
	}
	snap.ID = id
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, err
	}
	snapPath := filepath.Join(dir, id+".json")
	if err := writeAtomic(snapPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("write snapshot: %w", err)
	}
	res.ID, res.Snapshot = id, snapPath

	fail := func(what string, cause error) (*Result, error) {
		msg := fmt.Sprintf("apply failed: %s: %v", what, cause)
		if rerr := rollbackSnapshot(snap, opts.IO); rerr != nil {
			return res, fmt.Errorf("%s; AUTOMATIC ROLLBACK FAILED (%w); run: kerneltuner rollback %s", msg, rerr, id)
		}
		return res, fmt.Errorf("%s; rolled back snapshot %s", msg, id)
	}

	// 2. Managed files.
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fail("create dir for "+p, err)
		}
		mode := os.FileMode(0o644)
		for _, f := range snap.Files {
			if f.Path == p && !f.DidNotExist {
				mode = os.FileMode(f.Mode)
			}
		}
		if err := writeAtomic(p, newContent[p], mode); err != nil {
			return fail("write "+p, err)
		}
	}
	// 3. Live values.
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		if err := opts.IO.Write(op.LivePath, op.Proposed+"\n"); err != nil {
			return fail("write live value "+op.LivePath, err)
		}
	}
	// 4. Verify.
	for _, p := range paths {
		got, err := os.ReadFile(p)
		if err != nil || string(got) != string(newContent[p]) {
			return fail("verify "+p, errors.New("file content differs from what was written"))
		}
	}
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		got, err := opts.IO.Read(op.LivePath)
		if err != nil {
			return fail("verify "+op.LivePath, err)
		}
		if !valueMatches(got, op.Proposed) {
			return fail("verify "+op.LivePath, fmt.Errorf("wanted %q, kernel reports %q", norm(op.Proposed), norm(got)))
		}
	}
	return res, nil
}

// Rollback restores the snapshot id exactly. It is idempotent.
func Rollback(root facts.Root, id string) error {
	return RollbackWith(root, id, OSIO)
}

// RollbackWith is Rollback with an explicit ValueIO.
func RollbackWith(root facts.Root, id string, io ValueIO) error {
	snap, err := load(root, id)
	if err != nil {
		return err
	}
	return rollbackSnapshot(snap, io)
}

func rollbackSnapshot(s *Snapshot, io ValueIO) error {
	var errs []error
	for _, lv := range s.Live {
		if err := io.Write(lv.Path, restoreValue(lv.Prior)); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", lv.Path, err))
		}
	}
	for _, f := range s.Files {
		if f.DidNotExist {
			if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("remove %s: %w", f.Path, err))
			}
			continue
		}
		if err := writeAtomic(f.Path, f.Content, os.FileMode(f.Mode)); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", f.Path, err))
		}
	}
	dirs := append([]string(nil), s.CreatedDirs...)
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		_ = os.Remove(d) // only succeeds when empty
	}
	return errors.Join(errs...)
}

// List returns the snapshots under root, oldest first.
func List(root facts.Root) ([]Snapshot, error) {
	ents, err := os.ReadDir(snapDir(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		s, err := load(root, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func load(root facts.Root, id string) (*Snapshot, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("invalid snapshot id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(snapDir(root), id+".json"))
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	return &s, nil
}

// writeAtomic writes via a temp file in the same directory plus rename.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kerneltuner-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// missingDirs lists dir and its ancestors that do not exist yet.
func missingDirs(dir string) []string {
	var out []string
	for dir != "" && dir != filepath.Dir(dir) {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		out = append(out, dir)
		dir = filepath.Dir(dir)
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// valueMatches compares with whitespace normalisation. Selector-style files
// such as "[none] mq-deadline" match when the bracketed token equals want.
func valueMatches(got, want string) bool {
	if norm(got) == norm(want) {
		return true
	}
	return bracketed(got) != "" && bracketed(got) == norm(want)
}

func bracketed(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.IndexByte(s, ']')
	if i < 0 || j < i {
		return ""
	}
	return s[i+1 : j]
}

// restoreValue returns what to write back for a recorded raw value: the raw
// content, except for selector files where only the active token is writable.
func restoreValue(raw string) string {
	if b := bracketed(raw); b != "" {
		return b + "\n"
	}
	return raw
}

// merge produces the new content of a managed file, replacing lines for the
// keys being set and preserving everything else.
func merge(kind, existing string, isNew bool, ops []Op) []byte {
	sep, header := " = ", "# Managed by kerneltuner. Remove with: kerneltuner rollback <id>\n"
	if kind == fileDropin {
		sep, header = "=", "# Managed by kerneltuner.\n"
	}
	want := map[string]string{}
	var order []string
	for _, op := range ops {
		if _, ok := want[op.FileKey]; !ok {
			order = append(order, op.FileKey)
		}
		want[op.FileKey] = op.Proposed
	}
	var lines []string
	if existing != "" {
		lines = strings.Split(strings.TrimSuffix(existing, "\n"), "\n")
	}
	done := map[string]bool{}
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || t[0] == '#' || t[0] == ';' {
			out = append(out, ln)
			continue
		}
		k, _, ok := strings.Cut(t, "=")
		k = strings.TrimSpace(k)
		if ok {
			if v, hit := want[k]; hit {
				if !done[k] { // drop duplicates of a managed key
					out = append(out, k+sep+v)
					done[k] = true
				}
				continue
			}
		}
		out = append(out, ln)
	}
	var add []string
	for _, k := range order {
		if !done[k] {
			add = append(add, k+sep+want[k])
		}
	}
	if kind == fileDropin && len(add) > 0 {
		idx := -1
		for i, ln := range out {
			if strings.TrimSpace(ln) == "[Service]" {
				idx = i
			}
		}
		if idx < 0 {
			out = append(out, "[Service]")
			out = append(out, add...)
		} else {
			out = append(out[:idx+1], append(add, out[idx+1:]...)...)
		}
	} else {
		out = append(out, add...)
	}
	if isNew || existing == "" {
		out = append([]string{strings.TrimSuffix(header, "\n")}, out...)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
