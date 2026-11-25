//go:build !windows && !linux && !darwin

package main

import "errors"

func groupProcesses(int) ([]groupProcess, error) { return nil, errors.New("unsupported") }
func processArguments(int) ([]string, error)     { return nil, errors.New("unsupported") }
func adoptBrowserChildren() error                { return errors.New("unsupported") }
func reapBrowserChild(int)                       {}
