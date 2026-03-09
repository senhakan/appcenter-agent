//go:build !windows

package main

import "log"

func ensureRemoteSupportFirewallRules(_ string, _ *log.Logger) {}
