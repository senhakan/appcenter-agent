//go:build !windows

package main

import "log"

type traySupervisor struct{}

func newTraySupervisor(_ string, _ *log.Logger) *traySupervisor {
	return &traySupervisor{}
}

func (t *traySupervisor) SetEnabled(_ bool) {}
