package facts

import "path/filepath"

// Root locates the kernel pseudo-filesystems. Nothing may hard-code /proc.
type Root struct{ Proc, Sys, Etc, Run, Var string }

// DefaultRoot returns the real system paths.
func DefaultRoot() Root {
	return Root{Proc: "/proc", Sys: "/sys", Etc: "/etc", Run: "/run", Var: "/var"}
}

// RootAt returns a Root rooted at dir (dir/proc, dir/sys, ...).
func RootAt(dir string) Root {
	return Root{
		Proc: filepath.Join(dir, "proc"),
		Sys:  filepath.Join(dir, "sys"),
		Etc:  filepath.Join(dir, "etc"),
		Run:  filepath.Join(dir, "run"),
		Var:  filepath.Join(dir, "var"),
	}
}

// Facts is the static host snapshot.
type Facts struct {
	Kernel     Kernel
	Host       Host
	CPU        CPU
	NUMA       []NUMANode
	Memory     Memory
	NICs       []NIC
	Block      []BlockDev
	Accel      []Accelerator
	RDMA       []RDMADev
	IOMMU      IOMMU
	Sysctl     map[string]string // dotted name -> value, whitespace normalised to single spaces
	Limits     Limits
	Modules    map[string]bool // /proc/modules names
	Procs      []Proc          // all processes
	Swaps      []Swap          // /proc/swaps
	Tuned      Tuned
	Bpftune    bool // a process with comm "bpftune" is running
	Irqbalance bool
	Warnings   []string
}

// Kernel describes the running kernel.
type Kernel struct {
	Release             string
	Major, Minor, Patch int
	Cmdline             string
	CmdlineArgs         map[string]string // "iommu"->"pt", flags -> ""
	AvailableCongestion []string          // net.ipv4.tcp_available_congestion_control
}

// Host describes virtualization, container and cloud environment.
type Host struct {
	Virt             string // "none","kvm","vmware","xen","hyperv","unknown"
	Container        bool
	ContainerRuntime string
	Cloud            string // "aws","gcp","azure","" from DMI
	Distro           Distro // from os-release
}

// Distro is the ID and VERSION_ID of os-release ("ubuntu", "24.04"). Both are
// empty when no os-release file is readable.
type Distro struct{ ID, VersionID string }

// Swap is one row of /proc/swaps.
type Swap struct {
	Path     string
	Type     string // "partition" | "file"
	SizeKB   uint64
	Priority int
	Zram     bool // Path is a /dev/zram* device
}

// CPU describes processor topology and power settings.
type CPU struct {
	Vendor, Model                            string
	Sockets, Cores, Threads                  int // Cores = physical
	SMTActive                                bool
	SMTControl                               string
	Flags                                    map[string]bool
	ScalingDriver, Governor, EPP, IdleDriver string
	// Vulnerabilities maps /sys/devices/system/cpu/vulnerabilities/<name> to its
	// text, e.g. "spectre_v2" -> "Mitigation: Retpolines; ...".
	Vulnerabilities map[string]string
	// IDs are the logical CPU ids present under /sys/devices/system/cpu (cpuN).
	IDs []int
}

// NUMANode is one NUMA node.
type NUMANode struct {
	ID            int
	CPUs          []int
	MemTotalBytes uint64
}

// Memory describes RAM, swap, THP and hugepages.
type Memory struct {
	TotalBytes, SwapTotalBytes uint64
	THPEnabled, THPDefrag      string            // bracketed active value, e.g. "madvise"
	HugePages                  map[string]uint64 // "2048kB"->count, "1048576kB"->count
}

// NIC is one network interface.
type NIC struct {
	Name, Driver       string
	SpeedMbps, MTU     int
	NUMANode           int
	Virtual            bool
	RxQueues, TxQueues int
	// 0 when unknown.
	Combined, CombinedMax, RxRing, RxRingMax, TxRing, TxRingMax int
}

// BlockDev is one block device.
type BlockDev struct {
	Name, Transport string // Transport: "nvme","sata","virtio",""
	Rotational      bool
	Scheduler       string
	Schedulers      []string
	ReadAheadKB     int
	NrRequests      int
}

// Accelerator is a GPU/NPU found via sysfs PCI.
type Accelerator struct {
	Vendor                                     string // "nvidia","amd","intel","habana"
	PCIAddr, VendorID, DeviceID, Class, Driver string
	NUMANode                                   int // -1 when unknown
	LinkSpeed, LinkSpeedMax                    string
	LinkWidth, LinkWidthMax                    int
	IOMMUGroup                                 string
	// Kind is "gpu", "nvswitch" or "npu".
	Kind string
}

// RDMADev is an RDMA device.
type RDMADev struct {
	Name  string
	Ports []RDMAPort
}

// RDMAPort is one port of an RDMA device.
type RDMAPort struct {
	Num                            int
	State, LinkLayer, Rate, NetDev string
}

// IOMMU describes IOMMU state.
type IOMMU struct {
	Enabled, Passthrough bool
	Groups               int
}

// Limits holds system-wide file limits.
type Limits struct{ FileMax, NrOpen, FileNrAllocated uint64 }

// Proc is one process. math.MaxUint64 means unlimited for limits.
type Proc struct {
	PID         int
	Comm        string
	Cmdline     []string
	NoFileSoft  uint64
	NoFileHard  uint64
	MemlockSoft uint64
	MemlockHard uint64
	OpenFDs     int
	Unit        string // systemd unit from /proc/<pid>/cgroup, "" if none
	Container   string // container runtime from the cgroup path: docker|containerd|crio|podman, "" if none
}

// Tuned describes the tuned daemon state.
type Tuned struct {
	Active  bool
	Profile string
}
