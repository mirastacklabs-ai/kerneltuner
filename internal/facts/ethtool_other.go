//go:build !linux

package facts

import "errors"

// QueryEthtool is only available on Linux.
func QueryEthtool(string) (Ring, error) { return Ring{}, errors.New("ethtool: unsupported platform") }
