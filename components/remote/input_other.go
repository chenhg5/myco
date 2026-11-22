//go:build (!darwin && !windows) || (darwin && !cgo)

package main

import (
	"errors"
	"os/exec"
)

var keyCodes = map[string]uint16{"KeyA": 0, "Enter": 13}

func inputSupported() bool         { return false }
func displayGeometry() [4]int      { return [4]int{} }
func nativeInput(inputEvent) error { return errors.New("unsupported_platform") }
func hideProcess(*exec.Cmd)        {}

func requestInputPermission() {}

func nativeText(string) error { return errors.New("unsupported_platform") }
