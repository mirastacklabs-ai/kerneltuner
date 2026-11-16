package main

import "fmt"

// limitsFile overwrites /proc/<pid>/limits with explicit nofile, nproc and
// core values (unlimited is spelled out as in the kernel).
func (f *fx) limitsFile(pid int, nofile, nprocSoft, nprocHard, coreSoft string) {
	f.w(fmt.Sprintf("proc/%d/limits", pid), fmt.Sprintf(`Limit                     Soft Limit           Hard Limit           Units
Max cpu time              unlimited            unlimited            seconds
Max file size             unlimited            unlimited            bytes
Max data size             unlimited            unlimited            bytes
Max stack size            8388608              unlimited            bytes
Max core file size        %-20s unlimited            bytes
Max resident set          unlimited            unlimited            bytes
Max processes             %-20s %-20s processes
Max open files            %-20s %-20s files
Max locked memory         8388608              8388608              bytes
Max address space         unlimited            unlimited            bytes
`, coreSoft, nprocSoft, nprocHard, nofile, nofile))
}

// engineFacts2 exercises the second engine fact pass: the mount table with a
// device-mapper volume on a partition of an NVMe disk, a NFS share and a
// partition of a SATA disk, a kernel config, oom_score_adj, nproc and core
// limits, and a sysctl outside the collected directories.
func engineFacts2() *fx {
	f := newFx("engine-facts-2", "Engine fact coverage 2: mounts (ext4 root, xfs noatime LVM on nvme0n1p3, NFS, tmpfs, sdb1), boot kernel config, postgres with oom_score_adj -900 and nproc/core limits, user.max_user_namespaces outside the sysctl dirs", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro quiet")
	f.osRelease("ubuntu", "24.04")
	f.w("proc/mounts", "/dev/nvme0n1p2 / ext4 rw,relatime,errors=remount-ro 0 0\n"+
		"/dev/mapper/vg0-data /var/lib/postgresql xfs rw,noatime,attr2,inode64,logbufs=8 0 0\n"+
		"fileserver:/export /srv/media nfs4 rw,relatime,vers=4.2,hard 0 0\n"+
		"tmpfs /dev/shm tmpfs rw,nosuid,nodev,size=1048576k 0 0\n"+
		"/dev/sdb1 /mnt/with\\040space ext4 rw,relatime 0 0\n"+
		"proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0\n")
	f.block("nvme0n1", false, "none", []string{"none", "mq-deadline"}, 128, 1023)
	f.block("sdb", true, "mq-deadline", []string{"none", "mq-deadline", "bfq"}, 128, 256)
	pci := "../../devices/pci0000:00/0000:00:1d.0/0000:3b:00.0/nvme/nvme0/nvme0n1"
	f.ln("sys/class/block/nvme0n1", pci)
	f.ln("sys/class/block/nvme0n1p2", pci+"/nvme0n1p2")
	f.ln("sys/class/block/nvme0n1p3", pci+"/nvme0n1p3")
	f.ln("sys/class/block/sdb", "../../devices/pci0000:00/0000:00:1f.2/ata2/host1/target1:0:0/1:0:0:0/block/sdb")
	f.ln("sys/class/block/sdb1", "../../devices/pci0000:00/0000:00:1f.2/ata2/host1/target1:0:0/1:0:0:0/block/sdb/sdb1")
	f.ln("sys/class/block/dm-0", "../../devices/virtual/block/dm-0")
	f.wl("sys/block/dm-0/dm/name", "vg0-data")
	f.ln("sys/block/dm-0/slaves/nvme0n1p3", "../../../../devices/pci0000:00/0000:00:1d.0/0000:3b:00.0/nvme/nvme0/nvme0n1/nvme0n1p3")
	f.w("boot/config-6.8.0-45-generic", "# Automatically generated file\nCONFIG_BPF=y\nCONFIG_BPF_SYSCALL=y\nCONFIG_BPF_JIT=y\n"+
		"# CONFIG_BPF_JIT_ALWAYS_ON is not set\nCONFIG_HZ=250\nCONFIG_DEFAULT_TCP_CONG=\"cubic\"\nCONFIG_TCP_CONG_BBR=m\n"+
		"CONFIG_PREEMPT_DYNAMIC=y\n# CONFIG_NO_HZ_FULL is not set\n")
	f.sysctl(map[string]string{"user.max_user_namespaces": "15000"})
	f.proc(procSpec{pid: 3100, comm: "postgres", argv: []string{"/usr/lib/postgresql/16/bin/postgres", "-D", "/var/lib/postgresql/16/main"},
		nofile: "1024", unit: "postgresql@16-main.service", fds: 6})
	f.limitsFile(3100, "1024", "4096", "8192", "unlimited")
	f.wl("proc/3100/oom_score_adj", "-900")
	f.proc(procSpec{pid: 3101, comm: "postgres", argv: []string{"postgres: 16/main: checkpointer"},
		nofile: "1024", unit: "postgresql@16-main.service", fds: 3})
	f.limitsFile(3101, "1024", "4096", "8192", "0")
	f.wl("proc/3101/oom_score_adj", "0")
	f.listeners(22, 5432)
	return f
}
