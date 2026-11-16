package facts

import (
	"os"
	"path/filepath"
	"strings"
)

func collectHost(r Root, f *Facts) {
	h := &f.Host
	dmi := filepath.Join(r.Sys, "class/dmi/id")
	vendor := readOpt(filepath.Join(dmi, "sys_vendor"))
	product := readOpt(filepath.Join(dmi, "product_name"))
	biosVendor := readOpt(filepath.Join(dmi, "bios_vendor"))
	assetTag := readOpt(filepath.Join(dmi, "chassis_asset_tag"))
	if vendor == "" && product == "" {
		f.warnf("no DMI data under %s", dmi)
	}

	collectDistro(r, f)
	h.Virt, h.Cloud = classifyDMI(vendor, product, biosVendor, assetTag, f.CPU.Flags["hypervisor"])

	// Container signals. /.dockerenv sits next to the root's etc directory.
	rootDir := filepath.Dir(r.Etc)
	cgroup := readOpt(filepath.Join(r.Proc, "1/cgroup"))
	environ := containerEnv(filepath.Join(r.Proc, "1/environ"))
	switch {
	case strings.Contains(cgroup, "cri-containerd"):
		h.Container, h.ContainerRuntime = true, "containerd"
	case strings.Contains(cgroup, "crio-"):
		h.Container, h.ContainerRuntime = true, "crio"
	case strings.Contains(cgroup, "/docker/") || strings.Contains(cgroup, "docker-"):
		h.Container, h.ContainerRuntime = true, "docker"
	case strings.Contains(cgroup, "kubepods"):
		h.Container, h.ContainerRuntime = true, "kubernetes"
	case strings.Contains(cgroup, "/lxc/") || strings.Contains(cgroup, "lxc.payload"):
		h.Container, h.ContainerRuntime = true, "lxc"
	case environ != "":
		h.Container, h.ContainerRuntime = true, environ
	case exists(filepath.Join(r.Run, ".containerenv")):
		h.Container, h.ContainerRuntime = true, "podman"
	case exists(filepath.Join(rootDir, ".dockerenv")):
		h.Container, h.ContainerRuntime = true, "docker"
	}
}

// classifyDMI derives virtualization and cloud from DMI strings.
func classifyDMI(vendor, product, bios, assetTag string, hypervisorFlag bool) (virt, cloud string) {
	all := strings.ToLower(vendor + "|" + product + "|" + bios)
	switch {
	case strings.Contains(all, "amazon ec2") || strings.Contains(strings.ToLower(bios), "amazon"):
		cloud = "aws"
	case strings.Contains(all, "google"):
		cloud = "gcp"
	case strings.Contains(assetTag, "7783-7084-3265-9085-8269-3286-77"):
		cloud = "azure"
	}
	switch {
	case strings.Contains(all, "vmware"):
		virt = "vmware"
	case strings.Contains(all, "xen"):
		virt = "xen"
	case strings.Contains(all, "microsoft") && strings.Contains(strings.ToLower(product), "virtual machine"):
		virt = "hyperv"
	case strings.Contains(all, "qemu") || strings.Contains(all, "kvm") ||
		strings.Contains(all, "amazon ec2") || strings.Contains(all, "google compute engine") ||
		strings.Contains(all, "openstack") || strings.Contains(all, "bochs") || strings.Contains(all, "apple virtualization"):
		virt = "kvm"
	case hypervisorFlag:
		virt = "unknown"
	default:
		virt = "none"
	}
	return virt, cloud
}

// containerEnv returns the container= value from a NUL-separated environ file.
func containerEnv(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, kv := range strings.Split(string(b), "\x00") {
		if v, ok := strings.CutPrefix(kv, "container="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// collectDistro reads ID and VERSION_ID from <Etc>/os-release, falling back to
// <root>/usr/lib/os-release (the root directory is the parent of Etc).
func collectDistro(r Root, f *Facts) {
	for _, p := range []string{
		filepath.Join(r.Etc, "os-release"),
		filepath.Join(filepath.Dir(r.Etc), "usr/lib/os-release"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f.Host.Distro = parseOSRelease(string(b))
		return
	}
}

func parseOSRelease(s string) Distro {
	var d Distro
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "ID":
			d.ID = strings.ToLower(v)
		case "VERSION_ID":
			d.VersionID = v
		}
	}
	return d
}
