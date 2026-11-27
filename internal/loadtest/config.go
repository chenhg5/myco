// Package loadtest implements a bounded, isolated Hub workload.
package loadtest

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Users          []int         `json:"users"`
	Repeats        int           `json:"repeats"`
	Devices        int           `json:"devices_per_user"`
	Agents         int           `json:"agents_per_device"`
	SeedMessages   int           `json:"seed_messages"`
	Warmup         time.Duration `json:"warmup_ns"`
	Duration       time.Duration `json:"duration_ns"`
	Cooldown       time.Duration `json:"cooldown_ns"`
	TaskEvery      time.Duration `json:"active_agent_task_every_ns"`
	TaskWork       time.Duration `json:"simulated_work_ns"`
	AppEvery       time.Duration `json:"app_refresh_every_ns"`
	RuntimeEvery   time.Duration `json:"runtime_every_ns"`
	RequestTimeout time.Duration `json:"request_timeout_ns"`
	Workers        int           `json:"short_request_workers"`
	Queue          int           `json:"scheduler_queue_limit"`
	Seed           int64         `json:"random_seed"`
	Output         string        `json:"-"`
	Profile        bool          `json:"profile"`
}

func Main(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 1 && args[0] == "serve-child" {
		return serveChild(ctx, in, out, errOut)
	}
	f := flag.NewFlagSet("myco-loadtest", flag.ContinueOnError)
	f.SetOutput(errOut)
	c := Config{Devices: 2, Agents: 5}
	var users string
	f.StringVar(&users, "users", "1,5,20", "sequential user tiers; each gets a fresh local Hub")
	f.IntVar(&c.Repeats, "repeat", 3, "independent repetitions per tier")
	f.IntVar(&c.SeedMessages, "seed-messages", 1000, "synthetic historical messages per Hub")
	f.DurationVar(&c.Warmup, "warmup", 2*time.Minute, "warmup, reported separately")
	f.DurationVar(&c.Duration, "duration", 10*time.Minute, "measured arrival window")
	f.DurationVar(&c.Cooldown, "cooldown", 2*time.Minute, "maximum task drain time")
	f.DurationVar(&c.TaskEvery, "task-every", time.Minute, "task arrival interval per active agent (20% active)")
	f.DurationVar(&c.TaskWork, "task-work", 5*time.Second, "simulated task work, no model calls")
	f.DurationVar(&c.AppEvery, "app-every", 5*time.Second, "foreground refresh interval")
	f.DurationVar(&c.RuntimeEvery, "runtime-every", 30*time.Second, "runtime report interval")
	f.DurationVar(&c.RequestTimeout, "request-timeout", 10*time.Second, "short request deadline")
	f.IntVar(&c.Workers, "workers", 64, "bounded short-operation workers")
	f.IntVar(&c.Queue, "queue", 1024, "maximum scheduled operations waiting in client")
	f.Int64Var(&c.Seed, "seed", 1, "fixture and scheduling seed")
	f.StringVar(&c.Output, "out", "", "new output directory, must not exist")
	f.BoolVar(&c.Profile, "profile", false, "collect child CPU/heap profiles; affects overhead")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments; external target URLs are not supported")
	}
	for _, v := range strings.Split(users, ",") {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			return errors.New("users must be integers 1..100")
		}
		c.Users = append(c.Users, n)
	}
	if e := c.validate(); e != nil {
		return e
	}
	return Run(ctx, c, out)
}
func (c Config) validate() error {
	if c.Output == "" {
		return errors.New("--out must name a new directory")
	}
	if c.Repeats < 1 || c.Repeats > 10 || c.Devices != 2 || c.Agents != 5 || len(c.Users) == 0 || len(c.Users) > 10 {
		return errors.New("invalid tier configuration")
	}
	for _, n := range c.Users {
		if n < 1 || n > 100 {
			return errors.New("user tier out of range")
		}
	}
	if c.Seed < 0 {
		return errors.New("seed must be nonnegative")
	}
	if c.SeedMessages < 0 || c.SeedMessages > 1000000 || c.Workers < 1 || c.Workers > 512 || c.Queue < 1 || c.Queue > 100000 {
		return errors.New("invalid fixture or scheduler bounds")
	}
	if c.Warmup < 0 || c.Warmup > time.Hour || c.Duration <= 0 || c.Duration > 2*time.Hour || c.Cooldown <= 0 || c.Cooldown > 10*time.Minute {
		return errors.New("invalid phase durations")
	}
	for _, d := range []time.Duration{c.TaskEvery, c.AppEvery, c.RuntimeEvery, c.RequestTimeout} {
		if d <= 0 || d > time.Hour {
			return errors.New("intervals must be positive and <=1h")
		}
	}
	if c.TaskWork < 0 || c.TaskWork > time.Minute {
		return errors.New("task-work must be between 0 and 1m")
	}
	// Bound retained task state and the total scheduled operation budget.
	maxUsers := 0
	for _, n := range c.Users {
		maxUsers = max(maxUsers, n)
	}
	secs := (c.Warmup + c.Duration).Seconds()
	taskCount := float64(maxUsers*2) * secs / c.TaskEvery.Seconds()
	operations := taskCount + float64(maxUsers*20)*secs/c.RuntimeEvery.Seconds() + float64(maxUsers*2)*secs/c.AppEvery.Seconds()
	if taskCount > 100000 || operations > 2000000 {
		return errors.New("scenario exceeds bounded task/operation budget; reduce duration or arrival rate")
	}
	return nil
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func newRoot(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	if e = os.Mkdir(p, 0700); e != nil {
		return "", fmt.Errorf("create fresh output directory: %w", e)
	}
	return p, nil
}
