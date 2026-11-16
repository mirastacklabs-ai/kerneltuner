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
	"syscall"
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
	b, err := facts.ReadBounded(p, 1<<20)
	return string(b), err
}

func (osIO) Write(p, v string) error {
	// Never create live files, never follow a symlink at the final component.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0)
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
	// RolledBackAt is set once the snapshot has been restored; rollback with
	// no ID skips such snapshots.
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty"`
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

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
	root := plan.Root
	for _, op := range plan.Ops {
		if err := checkOp(root, op); err != nil {
			return nil, fmt.Errorf("refusing op %s (rule %s): %w", facts.SafeText(op.Param), facts.SafeText(op.RuleID), err)
		}
	}
	if len(plan.Blocked) > 0 && !opts.DryRun {
		return nil, fmt.Errorf("refusing to apply conflicting rules (select one with --rule): %s", strings.Join(plan.Blocked, "; "))
	}
	res := &Result{DryRun: opts.DryRun, Ops: plan.Ops}
	units := map[string]bool{}
	for _, op := range plan.Ops {
		if op.Unit != "" && !units[op.Unit] {
			units[op.Unit] = true
			res.Hints = append(res.Hints, "limits take effect after: systemctl daemon-reload && systemctl restart "+shellQuote(op.Unit))
		}
	}
	if opts.DryRun || len(plan.Ops) == 0 {
		return res, nil
	}

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
	live := map[string]string{} // op.LivePath -> resolved path
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		rp, err := resolveLive(root, op.LivePath)
		if err != nil {
			return nil, fmt.Errorf("live path %s: %w", op.LivePath, err)
		}
		live[op.LivePath] = rp
		prior, err := opts.IO.Read(rp)
		if err != nil {
			return nil, fmt.Errorf("read live value %s: %w", op.LivePath, err)
		}
		snap.Live = append(snap.Live, LiveValue{Path: op.LivePath, Prior: prior})
	}
	newContent := map[string][]byte{}
	for _, p := range paths {
		fsState := FileState{Path: p}
		prior, meta, err := readFileBeneath(root.Etc, p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fsState.DidNotExist = true
			snap.CreatedDirs = append(snap.CreatedDirs, missingDirs(root.Etc, filepath.Dir(p))...)
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", p, err)
		default:
			fsState.Content = prior
			fsState.Mode = meta.Mode & 0o644
		}
		newContent[p] = merge(edits[p][0].FileKind, string(prior), fsState.DidNotExist, edits[p])
		snap.Files = append(snap.Files, fsState)
	}
	snap.CreatedDirs = dedupe(snap.CreatedDirs)

	// 1. Snapshot first.
	dir := snapDir(root)
	for _, d := range []string{filepath.Dir(dir), dir} {
		if err := ensurePrivateDir(root.Var, d); err != nil {
			return nil, fmt.Errorf("snapshot dir: %w", err)
		}
	}
	taken := map[string]bool{}
	if ents, err := listDirBeneath(root.Var, dir); err == nil {
		for _, e := range ents {
			taken[e.Name()] = true
		}
	}
	id := now.Format("20060102T150405Z")
	for n := 2; taken[id+".json"]; n++ {
		id = fmt.Sprintf("%s-%d", now.Format("20060102T150405Z"), n)
	}
	snap.ID = id
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, err
	}
	snapPath := filepath.Join(dir, id+".json")
	if err := writeFileBeneath(root.Var, snapPath, data, 0o600, nil); err != nil {
		return nil, fmt.Errorf("write snapshot: %w", err)
	}
	res.ID, res.Snapshot = id, snapPath

	fail := func(what string, cause error) (*Result, error) {
		msg := fmt.Sprintf("apply failed: %s: %v", what, cause)
		if rerr := rollbackSnapshot(root, snap, opts.IO); rerr != nil {
			return res, fmt.Errorf("%s; AUTOMATIC ROLLBACK FAILED (%w); run: kerneltuner rollback %s", msg, rerr, id)
		}
		if merr := markRolledBack(root, snap); merr != nil {
			return res, fmt.Errorf("%s; rolled back snapshot %s (could not mark it: %w)", msg, id, merr)
		}
		return res, fmt.Errorf("%s; rolled back snapshot %s", msg, id)
	}

	// 2. Managed files.
	for _, p := range paths {
		mode := os.FileMode(0o644)
		for _, f := range snap.Files {
			if f.Path == p && !f.DidNotExist {
				mode = os.FileMode(f.Mode)
			}
		}
		if err := writeFileBeneath(root.Etc, p, newContent[p], mode, nil); err != nil {
			return fail("write "+p, err)
		}
	}
	// 3. Live values.
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		if err := opts.IO.Write(live[op.LivePath], op.Proposed+"\n"); err != nil {
			return fail("write live value "+op.LivePath, err)
		}
	}
	// 4. Verify.
	for _, p := range paths {
		got, _, err := readFileBeneath(root.Etc, p)
		if err != nil || string(got) != string(newContent[p]) {
			return fail("verify "+p, errors.New("file content differs from what was written"))
		}
	}
	for _, op := range plan.Ops {
		if op.LivePath == "" {
			continue
		}
		got, err := opts.IO.Read(live[op.LivePath])
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
	if err := rollbackSnapshot(root, snap, io); err != nil {
		return err
	}
	if err := markRolledBack(root, snap); err != nil {
		return fmt.Errorf("snapshot %s restored but could not be marked rolled back: %w", id, err)
	}
	return nil
}

// markRolledBack records the rollback time in the snapshot file.
func markRolledBack(root facts.Root, s *Snapshot) error {
	t := time.Now().UTC()
	s.RolledBackAt = &t
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileBeneath(root.Var, filepath.Join(snapDir(root), s.ID+".json"), data, 0o600, nil)
}

func rollbackSnapshot(root facts.Root, s *Snapshot, io ValueIO) error {
	var errs []error
	for _, lv := range s.Live {
		rp, err := resolveLive(root, lv.Path)
		if err == nil {
			err = io.Write(rp, restoreValue(lv.Prior))
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", lv.Path, err))
		}
	}
	for _, f := range s.Files {
		if f.DidNotExist {
			if err := removeBeneath(root.Etc, f.Path, false); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", f.Path, err))
			}
			continue
		}
		if err := writeFileBeneath(root.Etc, f.Path, f.Content, os.FileMode(f.Mode), nil); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", f.Path, err))
		}
	}
	dirs := append([]string(nil), s.CreatedDirs...)
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		base := root.Etc
		if !under(root.Etc, d) {
			base = root.Var
		}
		if filepath.Clean(d) == filepath.Clean(base) {
			if st, err := os.Lstat(d); err == nil && st.Mode()&os.ModeSymlink == 0 {
				_ = os.Remove(d) // only succeeds when empty
			}
			continue
		}
		_ = removeBeneath(base, d, true) // only succeeds when empty; never through a symlink
	}
	return errors.Join(errs...)
}

// List returns the snapshots under root, oldest first. Corrupt or invalid
// files are skipped with a warning on stderr.
func List(root facts.Root) ([]Snapshot, error) {
	return ListWarn(root, func(m string) { fmt.Fprintln(os.Stderr, "warning:", m) })
}

// ListWarn is List with a warning callback.
func ListWarn(root facts.Root, warn func(string)) ([]Snapshot, error) {
	ents, err := listDirBeneath(root.Var, snapDir(root))
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
			warn(fmt.Sprintf("skipping snapshot %s: %v", facts.SafeText(name), err))
			continue
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

// Latest returns the newest snapshot that has not been rolled back.
func Latest(snaps []Snapshot) (Snapshot, bool) {
	for i := len(snaps) - 1; i >= 0; i-- {
		if snaps[i].RolledBackAt == nil {
			return snaps[i], true
		}
	}
	return Snapshot{}, false
}

func load(root facts.Root, id string) (*Snapshot, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("invalid snapshot id %q", id)
	}
	dir := snapDir(root)
	if euid() == 0 {
		// As root we act on this file: it and its directories must be ours.
		for _, d := range []string{filepath.Dir(dir), dir} {
			m, err := statDirBeneath(root.Var, d)
			if err != nil {
				return nil, fmt.Errorf("snapshot %s: %w", id, err)
			}
			if err := trusted(d, m); err != nil {
				return nil, fmt.Errorf("snapshot %s: %w", id, err)
			}
		}
	}
	b, meta, err := readFileBeneath(root.Var, filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	if euid() == 0 {
		if err := trusted(id+".json", meta); err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", id, err)
		}
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	s.ID = id
	if err := validateSnapshot(root, &s); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	return &s, nil
}

// trusted requires root ownership and no group/world write access.
func trusted(what string, m fileMeta) error {
	if m.UID != 0 {
		return fmt.Errorf("%s is owned by uid %d, not root: refusing to trust it", what, m.UID)
	}
	if m.Mode&0o022 != 0 {
		return fmt.Errorf("%s is group- or world-writable (mode %04o): refusing to trust it", what, m.Mode)
	}
	return nil
}

// missingDirs lists dir and its ancestors that do not exist yet, stopping at
// etc (inclusive): nothing above it is ever recorded, so a snapshot stays
// valid for rollback (validateSnapshot) even on an empty --root.
func missingDirs(etc, dir string) []string {
	var out []string
	etc = filepath.Clean(etc)
	for under(etc, dir) {
		if _, err := os.Lstat(dir); err == nil {
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
