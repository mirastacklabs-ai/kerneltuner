package apply

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// sysfsAllow lists the sysfs files kerneltuner may write, relative to
// Root.Sys. "*" matches exactly one path element.
var sysfsAllow = []string{
	"block/*/queue/*",
	"devices/system/cpu/*/cpufreq/*",
	"kernel/mm/transparent_hugepage/*",
	"class/net/*/queues/*/rps_cpus",
	"class/net/*/queues/*/xps_cpus",
	"kernel/mm/hugepages/*/nr_hugepages",
}

// sysctlKeyRe is what a sysctl name may look like (it becomes a path element
// and a line key in sysctl.d, so no whitespace, "=" or control characters).
var sysctlKeyRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./:@+-]*$`)

func sysfsAllowed(rel string) bool {
	for _, pat := range sysfsAllow {
		if ok, _ := path.Match(pat, filepath.ToSlash(rel)); ok {
			return true
		}
	}
	return false
}

// checkValue rejects proposed values that could smuggle extra lines or
// control characters into a managed file or a kernel write.
func checkValue(v string) error {
	if v == "" {
		return errors.New("empty value")
	}
	for _, r := range v {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("value %q contains a newline or non-printable character", facts.SafeText(v))
		}
	}
	return nil
}

func under(base, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func dropinPath(root facts.Root, unit string) string {
	return filepath.Join(root.Etc, "systemd", "system", unit+".d", dropinFile)
}

func sysctlConfPath(root facts.Root) string { return filepath.Join(root.Etc, sysctlFile) }

// checkOp validates one operation independently of how it was built: the
// planner uses it to refuse hostile recommendations, Apply to refuse
// hand-made or tampered plans.
func checkOp(root facts.Root, op Op) error {
	if err := checkValue(op.Proposed); err != nil {
		return err
	}
	switch op.Kind {
	case "sysctl":
		if !sysctlKeyRe.MatchString(op.Param) || op.FileKey != op.Param {
			return fmt.Errorf("invalid sysctl name %q", facts.SafeText(op.Param))
		}
		if op.LivePath == "" || !under(filepath.Join(root.Proc, "sys"), op.LivePath) {
			return fmt.Errorf("live path %q is not under %s/sys", facts.SafeText(op.LivePath), root.Proc)
		}
		if op.File != sysctlConfPath(root) || op.FileKind != fileSysctl {
			return fmt.Errorf("unexpected managed file %q", facts.SafeText(op.File))
		}
	case "sysfs":
		if op.File != "" {
			return errors.New("sysfs ops do not write managed files")
		}
		rel, err := filepath.Rel(filepath.Clean(root.Sys), filepath.Clean(op.LivePath))
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("sysfs path %q is not under %s", facts.SafeText(op.LivePath), root.Sys)
		}
		if !sysfsAllowed(rel) {
			return fmt.Errorf("sysfs path /sys/%s is not on the allowlist of writable targets", facts.SafeText(rel))
		}
	case "limit":
		if !facts.ValidUnitName(op.Unit) {
			return fmt.Errorf("invalid systemd unit name %q", facts.SafeText(op.Unit))
		}
		switch op.FileKey {
		case "LimitNOFILE", "LimitMEMLOCK", "LimitNPROC":
		default:
			return fmt.Errorf("unsupported limit %q", facts.SafeText(op.FileKey))
		}
		if op.File != dropinPath(root, op.Unit) || op.FileKind != fileDropin || op.LivePath != "" {
			return fmt.Errorf("unexpected managed file %q", facts.SafeText(op.File))
		}
	default:
		return fmt.Errorf("unsupported op kind %q", facts.SafeText(op.Kind))
	}
	return nil
}

// resolveLive validates a live path and returns the path to read and write.
// It must lie lexically under Root.Proc/sys or Root.Sys; symlinks (such as
// /sys/block/<dev>) are resolved once and the result must still lie under
// the same tree. The final component is opened with O_NOFOLLOW by the IO.
func resolveLive(root facts.Root, p string) (string, error) {
	p = filepath.Clean(p)
	var base string
	switch {
	case under(filepath.Join(root.Proc, "sys"), p) && p != filepath.Join(root.Proc, "sys"):
		base = filepath.Join(root.Proc, "sys")
	case under(root.Sys, p) && p != filepath.Clean(root.Sys):
		base = root.Sys
	default:
		return "", fmt.Errorf("live path %q is not under %s/sys or %s", facts.SafeText(p), root.Proc, root.Sys)
	}
	if err := checkDangerousLive(root, p); err != nil {
		return "", err
	}
	rb, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	res, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	if !under(rb, res) || res == rb {
		return "", fmt.Errorf("live path %q resolves to %q outside %s", facts.SafeText(p), facts.SafeText(res), base)
	}
	if err := checkDangerousLive(root, res); err != nil {
		return "", err
	}
	if st, err := os.Lstat(res); err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("live path %q is not a regular file", facts.SafeText(res))
	}
	return res, nil
}

// managedFile reports whether p is one of the files kerneltuner owns under Etc.
func managedFile(root facts.Root, p string) bool {
	p = filepath.Clean(p)
	if p == sysctlConfPath(root) {
		return true
	}
	if filepath.Base(p) != dropinFile {
		return false
	}
	d := filepath.Dir(p)
	if filepath.Dir(d) != filepath.Join(root.Etc, "systemd", "system") {
		return false
	}
	name := filepath.Base(d)
	unit, ok := strings.CutSuffix(name, ".d")
	return ok && facts.ValidUnitName(unit)
}

// managedDir reports whether d may appear in a snapshot's CreatedDirs.
func managedDir(root facts.Root, d string) bool {
	d = filepath.Clean(d)
	etc := filepath.Clean(root.Etc)
	if d == etc || under(filepath.Join(etc, "sysctl.d"), d) {
		return true
	}
	if under(filepath.Join(etc, "systemd"), d) {
		return true
	}
	return under(filepath.Join(root.Var, "lib", "kerneltuner"), d)
}

// validateSnapshot checks everything a rollback will act on. Snapshots are
// JSON on disk and so untrusted. It also masks file modes.
func validateSnapshot(root facts.Root, s *Snapshot) error {
	for _, lv := range s.Live {
		if _, err := liveBase(root, lv.Path); err != nil {
			return err
		}
	}
	for i, f := range s.Files {
		if !managedFile(root, f.Path) {
			return fmt.Errorf("snapshot file %q is not a kerneltuner-managed file", facts.SafeText(f.Path))
		}
		s.Files[i].Mode = f.Mode & 0o644
	}
	for _, d := range s.CreatedDirs {
		if !managedDir(root, d) {
			return fmt.Errorf("snapshot directory %q is not one kerneltuner creates", facts.SafeText(d))
		}
	}
	return nil
}

// liveBase checks that p lies lexically under Proc/sys or Sys.
func liveBase(root facts.Root, p string) (string, error) {
	p = filepath.Clean(p)
	for _, b := range []string{filepath.Join(root.Proc, "sys"), root.Sys} {
		if under(b, p) && p != filepath.Clean(b) {
			return b, nil
		}
	}
	return "", fmt.Errorf("snapshot live path %q is not under %s/sys or %s", facts.SafeText(p), root.Proc, root.Sys)
}

// dangerousProc lists writable /proc/sys targets (relative to Proc/sys) whose
// value is executed or loaded by the kernel: writing them leads to running
// code, so they are never a tuning target. Paths ending in "/**" cover the
// whole subtree.
var dangerousProc = []string{
	"fs/binfmt_misc/**",
	"kernel/core_pattern",
	"kernel/modprobe",
	"kernel/hotplug",
	"kernel/poweroff_cmd",
}

// checkDangerousLive refuses p when it lies on the dangerousProc list.
func checkDangerousLive(root facts.Root, p string) error {
	rel, err := filepath.Rel(filepath.Join(root.Proc, "sys"), filepath.Clean(p))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	rel = filepath.ToSlash(rel)
	for _, d := range dangerousProc {
		if pre, ok := strings.CutSuffix(d, "/**"); ok {
			if rel == pre || strings.HasPrefix(rel, pre+"/") {
				return fmt.Errorf("live path /proc/sys/%s is a code-execution target and is never written", facts.SafeText(rel))
			}
		} else if rel == d {
			return fmt.Errorf("live path /proc/sys/%s is a code-execution target and is never written", facts.SafeText(rel))
		}
	}
	return nil
}
