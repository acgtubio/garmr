package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/acgtubio/garmr/internal/config"
	"github.com/acgtubio/garmr/internal/hosts"
	"github.com/acgtubio/garmr/internal/runner"
	"github.com/acgtubio/garmr/internal/schedule"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// now is replaced in tests.
var now = time.Now

const usage = `usage: garmr [flags] <command> [args]

commands:
  block add [-window HH:MM-HH:MM]... <site>...
                          add sites; -window (repeatable) gives a site its own
                          blocking times instead of the default schedule
  block remove <site>...  remove sites from the blocklist
  block list              show the blocklist and each site's windows
  schedule set HH:MM-HH:MM
                          set the default blocking window
  schedule clear          remove the default window (block all day)
  status                  show what is blocked right now
  run                     block on schedule, re-checking every minute (needs root)
  apply                   apply the schedule once for the current time (needs root)
  clear                   remove garmr's entries from the hosts file (needs root)

Windows use local time. A window like 22:00-06:00 runs past midnight;
00:00-00:00 covers the whole day.

flags:
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "garmr:", err)
		os.Exit(1)
	}
}

type app struct {
	configPath, hostsPath, lockPath string
	stdout                          io.Writer
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("garmr", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), usage)
		flags.PrintDefaults()
	}
	showVersion := flags.Bool("version", false, "print version and exit")
	configPath := flags.String("config", "", "config file path (default ~/.config/garmr/config.json)")
	hostsPath := flags.String("hosts", hosts.DefaultPath, "hosts file to manage")
	lockPath := flags.String("lock", runner.DefaultLockPath, "lock file held by 'garmr run'")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if *showVersion {
		fmt.Fprintf(stdout, "garmr %s\n", version)
		return nil
	}

	rest := flags.Args()
	if len(rest) == 0 {
		flags.SetOutput(stdout)
		flags.Usage()
		return nil
	}

	if *configPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return fmt.Errorf("determine config path: %w", err)
		}
		*configPath = p
	}
	a := &app{configPath: *configPath, hostsPath: *hostsPath, lockPath: *lockPath, stdout: stdout}

	cmd, cmdArgs := rest[0], rest[1:]
	switch cmd {
	case "block":
		return a.block(cmdArgs)
	case "schedule":
		return a.schedule(cmdArgs)
	case "status":
		return a.status()
	case "run":
		return a.run()
	case "apply":
		return a.apply()
	case "clear":
		return a.clear()
	default:
		return fmt.Errorf("unknown command %q (run 'garmr -h' for usage)", cmd)
	}
}

func (a *app) block(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: garmr block add|remove|list [site...]")
	}
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		for _, s := range cfg.Sites {
			fmt.Fprintf(a.stdout, "%-30s %s\n", s.Domain, windowsLabel(s.Windows))
		}
		return nil

	case "add":
		var windows windowList
		flags := flag.NewFlagSet("block add", flag.ContinueOnError)
		flags.SetOutput(a.stdout)
		flags.Var(&windows, "window", "blocking window `HH:MM-HH:MM` for these sites (repeatable)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		sites := flags.Args()
		if len(sites) == 0 {
			return errors.New("usage: garmr block add [-window HH:MM-HH:MM]... <site>...")
		}
		changed := false
		for _, s := range sites {
			d, ok, err := cfg.Add(s, windows)
			if err != nil {
				return err
			}
			changed = changed || ok
			switch {
			case ok && len(windows) > 0:
				fmt.Fprintf(a.stdout, "blocked: %s (%s)\n", d, schedule.Format(windows))
			case ok:
				fmt.Fprintf(a.stdout, "blocked: %s\n", d)
			default:
				fmt.Fprintf(a.stdout, "already blocked: %s\n", d)
			}
		}
		if changed {
			return cfg.Save(a.configPath)
		}
		return nil

	case "remove":
		sites := args[1:]
		if len(sites) == 0 {
			return errors.New("usage: garmr block remove <site>...")
		}
		changed := false
		for _, s := range sites {
			d, ok, err := cfg.Remove(s)
			if err != nil {
				return err
			}
			changed = changed || ok
			if ok {
				fmt.Fprintf(a.stdout, "unblocked: %s\n", d)
			} else {
				fmt.Fprintf(a.stdout, "not in blocklist: %s\n", d)
			}
		}
		if changed {
			return cfg.Save(a.configPath)
		}
		return nil

	default:
		return fmt.Errorf("unknown block subcommand %q (want add, remove or list)", args[0])
	}
}

func (a *app) schedule(args []string) error {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	switch {
	case len(args) == 2 && args[0] == "set":
		w, err := schedule.ParseWindow(args[1])
		if err != nil {
			return err
		}
		cfg.Schedule = &w
		fmt.Fprintf(a.stdout, "default schedule: %s\n", w)
	case len(args) == 1 && args[0] == "clear":
		cfg.Schedule = nil
		fmt.Fprintln(a.stdout, "default schedule cleared: sites without their own windows are blocked all day")
	default:
		return errors.New("usage: garmr schedule set HH:MM-HH:MM | garmr schedule clear")
	}
	return cfg.Save(a.configPath)
}

func (a *app) status() error {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	t := now()
	fmt.Fprintf(a.stdout, "time:     %s\n", t.Format("15:04 MST"))
	if cfg.Schedule != nil {
		fmt.Fprintf(a.stdout, "default:  %s\n", cfg.Schedule)
	} else {
		fmt.Fprintln(a.stdout, "default:  all day")
	}
	if held, _ := runner.Held(a.lockPath); held {
		fmt.Fprintln(a.stdout, "runner:   active")
	} else {
		fmt.Fprintln(a.stdout, "runner:   not running")
	}
	fmt.Fprintln(a.stdout)
	active := cfg.ActiveSites(t)
	for _, s := range cfg.Sites {
		state := "allowed"
		if slices.Contains(active, s.Domain) {
			state = "BLOCKED"
		}
		fmt.Fprintf(a.stdout, "%-8s %-30s %s\n", state, s.Domain, windowsLabel(s.Windows))
	}
	return nil
}

func (a *app) run() error {
	lock, err := runner.Acquire(a.lockPath)
	if err != nil {
		return hostsErr(err)
	}
	defer lock.Release()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(a.stdout, "garmr %s: enforcing %s on %s (Ctrl-C to stop; blocks stay in place)\n", version, a.configPath, a.hostsPath)
	return runner.Run(ctx, a.configPath, a.hostsPath, a.stdout)
}

func (a *app) apply() error {
	if err := a.refuseIfRunning(); err != nil {
		return err
	}
	active, err := runner.Tick(a.configPath, a.hostsPath, now())
	if err != nil {
		return hostsErr(err)
	}
	if len(active) == 0 {
		fmt.Fprintf(a.stdout, "nothing scheduled to be blocked now; %s has no garmr entries\n", a.hostsPath)
		return nil
	}
	fmt.Fprintf(a.stdout, "blocking now: %s\n", strings.Join(active, " "))
	fmt.Fprintln(a.stdout, "note: this is a one-off; use 'garmr run' to follow the schedule. Browsers may cache DNS or use DNS-over-HTTPS.")
	return nil
}

func (a *app) clear() error {
	if err := a.refuseIfRunning(); err != nil {
		return err
	}
	if err := hosts.Clear(a.hostsPath); err != nil {
		return hostsErr(err)
	}
	fmt.Fprintf(a.stdout, "cleared garmr entries from %s\n", a.hostsPath)
	return nil
}

func (a *app) refuseIfRunning() error {
	held, err := runner.Held(a.lockPath)
	if err != nil {
		return fmt.Errorf("check lock %s: %w", a.lockPath, err)
	}
	if held {
		return errors.New("'garmr run' is active and manages the hosts file; stop it first")
	}
	return nil
}

func windowsLabel(ws []schedule.Window) string {
	if len(ws) == 0 {
		return "(default)"
	}
	return schedule.Format(ws)
}

// windowList is a repeatable -window flag.
type windowList []schedule.Window

func (w *windowList) String() string {
	if w == nil {
		return ""
	}
	return schedule.Format(*w)
}

func (w *windowList) Set(s string) error {
	win, err := schedule.ParseWindow(s)
	if err != nil {
		return err
	}
	*w = append(*w, win)
	return nil
}

func hostsErr(err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%w (try running with sudo)", err)
	}
	return err
}
