package rules

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

// EnvInput is the per-evaluation state that expression functions close over.
type EnvInput struct {
	Facts    *facts.Facts
	Workload *workload.Workload
	Values   map[string]float64
	Rates    map[string]float64
	Root     facts.Root
}

// Env is the expression environment (docs/DESIGN.md "Expression environment").
// Programs are compiled once against the Env type and run against a value
// built by NewEnv.
type Env struct {
	Facts    *facts.Facts       `expr:"facts"`
	Workload *workload.Workload `expr:"workload"`
	Current  any                `expr:"current"`
	Item     any                `expr:"item"`

	Kver       func(string) (bool, error) `expr:"kver"`
	Exists     func(string) bool          `expr:"exists"`
	Sysctl     func(string) string        `expr:"sysctl"`
	SysctlNum  func(string) float64       `expr:"sysctlNum"`
	Metric     func(string) float64       `expr:"metric"`
	Rate       func(string) float64       `expr:"rate"`
	HasApp     func(string) bool          `expr:"hasApp"`
	CPUFlag    func(string) bool          `expr:"cpuFlag"`
	AccelCount func(string) int           `expr:"accelCount"`
	Module     func(string) bool          `expr:"module"`
	// Numeric helpers take ints or floats and return an exact int when every
	// argument is an int (so 9223372036854775807 never goes through float64).
	Bytes          func(string) (any, error)        `expr:"bytes"`
	Clamp          func(x, lo, hi any) (any, error) `expr:"clamp"`
	Pow2Ceil       func(x any) (any, error)         `expr:"pow2ceil"`
	Contains       func(list any, s any) bool       `expr:"contains_"`
	AccelKindCount func(vendor, kind string) int    `expr:"accelKindCount"`
	CmdlineHas     func(key, item string) bool      `expr:"cmdlineHas"`
	AppContainer   func(app string) string          `expr:"appContainer"`
	MountOf        func(path string) facts.Mount    `expr:"mountOf"`
	FsType         func(path string) string         `expr:"fsType"`
	MountHasOpt    func(path, opt string) bool      `expr:"mountHasOpt"`
	BlockOf        func(path string) string         `expr:"blockOf"`
	Kconfig        func(name string) string         `expr:"kconfig"`
	// Unlimited is math.MaxUint64, the value of an "unlimited" rlimit. The expr
	// parser rejects integer literals above int64, so rules use this instead.
	Unlimited   uint64           `expr:"unlimited"`
	IsUnlimited func(x any) bool `expr:"isUnlimited"`
}

// NewEnv builds the runtime environment. current and item may be nil.
func NewEnv(in EnvInput, current, item any) Env {
	if in.Facts == nil {
		in.Facts = &facts.Facts{}
	}
	if in.Workload == nil {
		in.Workload = &workload.Workload{}
	}
	f := in.Facts
	sysctl := func(name string) string {
		v, _ := SysctlValue(f, in.Root, name)
		return v
	}
	return Env{
		Facts: f, Workload: in.Workload, Current: current, Item: item,
		Kver: func(c string) (bool, error) {
			kc, err := ParseKernelConstraint(c)
			if err != nil {
				return false, err
			}
			if f.Kernel.Major == 0 && f.Kernel.Minor == 0 {
				return false, nil
			}
			return kc.Match(f.Kernel.Major, f.Kernel.Minor, f.Kernel.Patch), nil
		},
		Exists:    func(p string) bool { return existsUnder(in.Root, p) },
		Sysctl:    sysctl,
		SysctlNum: func(name string) float64 { return parseNum(sysctl(name)) },
		Metric:    func(k string) float64 { return in.Values[k] },
		Rate:      func(k string) float64 { return in.Rates[k] },
		HasApp:    in.Workload.Has,
		CPUFlag:   func(n string) bool { return f.CPU.Flags[n] },
		AccelCount: func(vendor string) int {
			n := 0 // NVSwitch bridges are not compute devices
			for _, a := range f.Accel {
				if (vendor == "" || a.Vendor == vendor) && a.Kind != "nvswitch" {
					n++
				}
			}
			return n
		},
		AccelKindCount: func(vendor, kind string) int {
			n := 0
			for _, a := range f.Accel {
				k := a.Kind
				if k == "" {
					k = "gpu"
				}
				if (vendor == "" || a.Vendor == vendor) && (kind == "" || k == kind) {
					n++
				}
			}
			return n
		},
		CmdlineHas: func(key, item string) bool { return CmdlineHas(f.Kernel.CmdlineArgs, key, item) },
		AppContainer: func(name string) string {
			for _, a := range in.Workload.Apps {
				if a.Name == name && a.Container != "" {
					return a.Container
				}
			}
			return ""
		},
		MountOf:     func(p string) facts.Mount { return f.MountOf(p) },
		FsType:      func(p string) string { return f.MountOf(p).FSType },
		MountHasOpt: func(p, opt string) bool { return f.MountOf(p).HasOpt(opt) },
		BlockOf:     func(p string) string { return f.BlockDeviceOf(p) },
		Kconfig:     func(name string) string { return f.Kernel.Config[name] },
		Unlimited:   math.MaxUint64,
		IsUnlimited: IsUnlimited,
		Module:      func(n string) bool { return f.Modules[n] },
		Bytes:       bytesFn,
		Clamp:       clampFn,
		Pow2Ceil:    pow2ceilFn,
		Contains:    containsFn,
	}
}

// SysctlValue returns the value of a dotted sysctl name: from Facts.Sysctl,
// else read from <Root.Proc>/sys/<path> (so names outside facts.SysctlDirs
// work). Whitespace is normalised to single spaces. ok is false when the
// sysctl cannot be read.
func SysctlValue(f *facts.Facts, r facts.Root, name string) (string, bool) {
	if v, ok := f.Sysctl[name]; ok {
		return v, true
	}
	if r.Proc == "" || name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(r.Proc, "sys", strings.ReplaceAll(name, ".", "/")))
	if err != nil {
		return "", false
	}
	return strings.Join(strings.Fields(string(b)), " "), true
}

// IsUnlimited reports whether x is an "unlimited" rlimit value: math.MaxUint64
// as uint64 or float (float64(MaxUint64) included), or the strings
// "unlimited" and "infinity".
func IsUnlimited(x any) bool {
	switch v := x.(type) {
	case uint64:
		return v == math.MaxUint64
	case uint:
		return uint64(v) == math.MaxUint64
	case float64:
		return v >= float64(math.MaxUint64)
	case float32:
		return float64(v) >= float64(math.MaxUint64)
	case string:
		t := strings.ToLower(strings.TrimSpace(v))
		return t == "unlimited" || t == "infinity"
	}
	return false
}

// CmdlineHas reports whether kernel cmdline key has item among its
// comma-separated values: pci=realloc=off,nommconf has "realloc=off". An
// empty item tests only that the key is present.
func CmdlineHas(args map[string]string, key, item string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	if item == "" || v == item {
		return true
	}
	for _, e := range strings.Split(v, ",") {
		if strings.TrimSpace(e) == item {
			return true
		}
	}
	return false
}

func containsFn(list, s any) bool {
	switch l := list.(type) {
	case []string:
		for _, e := range l {
			if fmt.Sprint(s) == e {
				return true
			}
		}
	case []any:
		for _, e := range l {
			if fmt.Sprint(e) == fmt.Sprint(s) {
				return true
			}
		}
	case string:
		return strings.Contains(l, fmt.Sprint(s))
	}
	return false
}

func parseNum(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// ParseBytes parses "4M", "64K", "1G", "4096" (binary units, case-insensitive,
// optional trailing B or iB).
func ParseBytes(s string) (float64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	t = strings.TrimSuffix(strings.TrimSuffix(t, "IB"), "B")
	mult := 1.0
	if t != "" {
		switch t[len(t)-1] {
		case 'K':
			mult, t = 1<<10, t[:len(t)-1]
		case 'M':
			mult, t = 1<<20, t[:len(t)-1]
		case 'G':
			mult, t = 1<<30, t[:len(t)-1]
		case 'T':
			mult, t = 1<<40, t[:len(t)-1]
		}
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil {
		return 0, fmt.Errorf("bytes(%q): invalid size", s)
	}
	return f * mult, nil
}

// existsUnder resolves an absolute path like /proc/x or /sys/x under the
// matching Root directory. Anything that does not start with a known root
// directory, or that would escape it, does not exist.
func existsUnder(r facts.Root, p string) bool {
	full, ok := ResolveUnder(r, p)
	if !ok {
		return false
	}
	_, err := os.Stat(full)
	return err == nil
}

// ResolveUnder maps /proc/..., /sys/..., /etc/..., /run/... or /var/... to the
// corresponding directory of r. The path is cleaned first, so it cannot
// escape the root.
func ResolveUnder(r facts.Root, p string) (string, bool) {
	c := filepath.Clean("/" + strings.TrimPrefix(p, "/"))
	for _, m := range []struct{ prefix, dir string }{
		{"/proc", r.Proc}, {"/sys", r.Sys}, {"/etc", r.Etc}, {"/run", r.Run}, {"/var", r.Var},
	} {
		if m.dir == "" {
			continue
		}
		if c == m.prefix {
			return m.dir, true
		}
		if strings.HasPrefix(c, m.prefix+"/") {
			return filepath.Join(m.dir, c[len(m.prefix):]), true
		}
	}
	return "", false
}

// TypedValue types a raw kernel value: float64 if it parses, []float64 for
// whitespace-separated numeric fields, else the (whitespace-normalised) string.
func TypedValue(s string) any {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	nums := make([]float64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return strings.Join(fields, " ")
		}
		nums = append(nums, v)
	}
	if len(nums) == 1 {
		return nums[0]
	}
	return nums
}

// contains is an operator keyword in expr, so the function form
// contains(list, s) is rewritten to the internal name contains_ before compiling.
var containsCall = regexp.MustCompile(`\bcontains\s*\(`)

type compiled struct {
	prog *vm.Program
	err  error
}

var progCache sync.Map // expression source -> *compiled

// Compile compiles src against Env. Programs are cached by source text, so
// each distinct expression is compiled once per process.
func Compile(src string) (*vm.Program, error) {
	if c, ok := progCache.Load(src); ok {
		cc := c.(*compiled)
		return cc.prog, cc.err
	}
	prog, err := expr.Compile(containsCall.ReplaceAllString(src, "contains_("), expr.Env(Env{}), expr.AsAny())
	progCache.Store(src, &compiled{prog, err})
	return prog, err
}

// Run executes a compiled program against env. Panics inside functions
// surface as errors.
func Run(prog *vm.Program, env Env) (out any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("expression panic: %v", p)
		}
	}()
	return expr.Run(prog, env)
}

// Eval compiles (cached) and runs src.
func Eval(src string, env Env) (any, error) {
	prog, err := Compile(src)
	if err != nil {
		return nil, err
	}
	return Run(prog, env)
}

// EvalBool is Eval requiring a boolean result.
func EvalBool(src string, env Env) (bool, error) {
	v, err := Eval(src, env)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q: want bool, got %T", src, v)
	}
	return b, nil
}

// ErrBadConstraint is wrapped by ParseKernelConstraint errors.
var ErrBadConstraint = errors.New("bad kernel constraint")

// KernelConstraint is a conjunction of version comparisons.
type KernelConstraint []kernelTerm

type kernelTerm struct {
	op     string
	v      [3]int
	nGiven int // how many of major.minor.patch were written
}

// ParseKernelConstraint parses space-separated terms like ">=4.9 <6.18".
// Operators: >= > <= < ==. Versions are major.minor[.patch].
func ParseKernelConstraint(s string) (KernelConstraint, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrBadConstraint)
	}
	var kc KernelConstraint
	for _, f := range fields {
		op := ""
		for _, o := range []string{">=", "<=", "==", ">", "<"} {
			if strings.HasPrefix(f, o) {
				op = o
				break
			}
		}
		if op == "" {
			return nil, fmt.Errorf("%w: %q needs one of >= > <= < ==", ErrBadConstraint, f)
		}
		parts := strings.Split(strings.TrimPrefix(f, op), ".")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("%w: %q wants major.minor[.patch]", ErrBadConstraint, f)
		}
		t := kernelTerm{op: op, nGiven: len(parts)}
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%w: %q has non-numeric version", ErrBadConstraint, f)
			}
			t.v[i] = n
		}
		kc = append(kc, t)
	}
	return kc, nil
}

// Match reports whether a kernel major.minor.patch satisfies every term.
func (kc KernelConstraint) Match(major, minor, patch int) bool {
	have := [3]int{major, minor, patch}
	for _, t := range kc {
		cmp := 0
		n := t.nGiven // only compare the components written: 5.4 matches every 5.4.x
		for i := 0; i < n && cmp == 0; i++ {
			switch {
			case have[i] < t.v[i]:
				cmp = -1
			case have[i] > t.v[i]:
				cmp = 1
			}
		}
		ok := false
		switch t.op {
		case ">=":
			ok = cmp >= 0
		case ">":
			ok = cmp > 0
		case "<=":
			ok = cmp <= 0
		case "<":
			ok = cmp < 0
		case "==":
			ok = cmp == 0
		}
		if !ok {
			return false
		}
	}
	return true
}
