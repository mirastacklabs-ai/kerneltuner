// Package deps pins dependencies that later waves will import for real.
// Delete each blank import once a package uses the module directly.
package deps

import (
	_ "github.com/coreos/go-systemd/v22/journal" // agent: journald
	_ "github.com/expr-lang/expr"                // rules: expression engine
	_ "github.com/prometheus/procfs"             // facts, metrics
	_ "github.com/safchain/ethtool"              // facts: NIC ring/channel ioctl
)
