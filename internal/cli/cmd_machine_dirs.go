package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/config"
)

// machineDirsCommand lists, adds and removes the folders new sessions may
// start in on this computer (spawn.dirs in machine.yaml). Like
// shell-access it is this computer's own policy: changed here, or by the
// myco app of someone who reaches this computer (this computer, over SSH,
// or into its WSL), never by a hub.
func machineDirsCommand() *Command {
	return &Command{Name: "dirs", Summary: "List, allow or stop allowing the folders agents may start in on this computer", Mutating: true,
		Long: `New sessions started on this computer (myco agent spawn, the app's 新建 agent) may only run in these
folders and the folders inside them: spawn.dirs in ~/.myco/machine.yaml. None written means all of the
home folder. --add allows a folder (one already inside an allowed folder is not added again); --remove
stops allowing one (the last one stays: none would mean all of home). The folder must exist. A change
applies to the next session started; sessions already running keep going.`,
		Examples: []string{"myco machine dirs", "myco machine dirs --add ~/code/tapnow", "myco machine dirs --remove ~/code/old --dry-run"},
		Flags: []Flag{
			{Name: "add", Kind: List, Help: "A folder to allow (~/folder or a full path; repeatable)"},
			{Name: "remove", Kind: List, Help: "A folder to stop allowing (repeatable)"},
		},
		Run: handler(func(e *env) error {
			m, err := config.LoadMachine(e.defaultPath)
			if err != nil {
				return err
			}
			if m == nil {
				return errorf("not_configured", ExitUsage, "join this computer with a link first: curl -fsSL <link> | bash",
					"this computer has not joined a hub as a machine yet")
			}
			home, _ := os.UserHomeDir()
			add, remove := e.in.List("add"), e.in.List("remove")
			if len(add) == 0 && len(remove) == 0 {
				dirs := config.EffectiveDirs(m.Spawn.Dirs)
				return e.p.Print(map[string]any{"dirs": dirs}, func(w io.Writer) {
					for _, d := range dirs {
						fmt.Fprintln(w, d)
					}
				})
			}
			for i, a := range add {
				d, err := config.CleanSpawnDir(a)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "pass a folder as ~/folder or a full path", "%v", err)
				}
				if st, err := os.Stat(config.ExpandDir(d, home)); err != nil || !st.IsDir() {
					return errorf("not_found", ExitNotFound, "create it first (mkdir -p "+d+"), or check the name", "folder %s does not exist on this computer", d)
				}
				add[i] = d
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "machine_dirs", "add": add, "remove": remove, "config": config.MachinePath(e.defaultPath)})
			}
			ch, err := config.SetSpawnDirs(e.defaultPath, home, add, remove)
			switch {
			case errors.Is(err, config.ErrNotJoined):
				return errorf("not_configured", ExitUsage, "join this computer with a link first: curl -fsSL <link> | bash", "%v", err)
			case errors.Is(err, config.ErrLastDir):
				return errorf("invalid_state", ExitConflict, "allow another folder first (--add), then remove this one", "%v", err)
			case err != nil:
				return errorf("invalid_argument", ExitUsage, "pass a folder as ~/folder or a full path", "%v", err)
			}
			out := map[string]any{"dirs": ch.Dirs, "changed": ch.Changed}
			if len(ch.Covered) > 0 {
				out["already_allowed"] = ch.Covered
			}
			if len(ch.Missing) > 0 {
				out["not_listed"] = ch.Missing
			}
			return e.p.Print(out, func(w io.Writer) {
				for _, d := range ch.Covered {
					fmt.Fprintf(w, "%s is already allowed (已经允许)\n", d)
				}
				for _, d := range ch.Missing {
					fmt.Fprintf(w, "%s was not in the list (不在列表里)\n", d)
				}
				fmt.Fprintln(w, "agents may start in (允许开工的文件夹):")
				for _, d := range ch.Dirs {
					fmt.Fprintln(w, "  "+d)
				}
			})
		}),
	}
}
