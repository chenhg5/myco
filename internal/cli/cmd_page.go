package cli

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// pageCommands publish web pages to the hub for people to open in the App
// (docs/design/browser.md).
func pageCommands() *Command {
	adminFile := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (people; agents use their own)"}
	pageFlag := Flag{Name: "page", Kind: String, Required: true, Help: "Page id (pg_...)"}
	publish := &Command{
		Name:     "publish",
		Summary:  "Publish a folder (or one html file) as a web page people open in the App",
		Mutating: true,
		Long: `Zips the folder and uploads it to the hub; prints the page's address. Write that address in
your reply: the App shows it as a card and opens the page in its panel, next to the chat.

The page is static files only (HTML, JS, CSS, pictures, data), opened in a sandbox: it cannot
reach the internet, so put everything it needs in the folder (no CDN links). It opens on
index.html (or --entry, or its only html file). .git, node_modules and hidden files are left out.
Publish the built output (e.g. dist/) of a project, not its sources.

Publishing again with --page makes a new version of the same page (people see "new version").
Up to 50 MB and 2000 files a version; 20 MB a file. With --project, the project's people see it;
otherwise your owner (and the workspace's admins).`,
		Examples: []string{
			`myco page publish --dir ./report --name "周报 · 第 46 周" --project growth`,
			"myco page publish --dir ./report --page pg_abc123",
			"myco page publish --dir chart.html --name 销售图表",
		},
		Flags: []Flag{
			{Name: "dir", Kind: String, Required: true, Help: "The folder to publish, or a single .html file"},
			{Name: "name", Kind: String, Help: "Its name, as people see it (required for a new page)"},
			{Name: "project", Kind: String, Help: "Publish it to this project (its people see it)"},
			{Name: "page", Kind: String, Help: "Publish a new version of this page (pg_...)"},
			{Name: "entry", Kind: String, Help: "The file it opens on (default index.html)"},
			adminFile,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("dir"); err != nil {
				return err
			}
			in := client.PageInput{Page: e.in.String("page"), Name: e.in.String("name"), Project: e.in.String("project"), Entry: e.in.String("entry")}
			if in.Page == "" && strings.TrimSpace(in.Name) == "" {
				return errorf("invalid_argument", ExitUsage, `give it a name people will recognise: --name "周报 · 第 46 周" (or --page to publish a new version)`, "a new page needs --name")
			}
			data, files, entry, err := zipPage(e.in.String("dir"))
			if err != nil {
				return err
			}
			if in.Entry == "" {
				in.Entry = entry
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "page_publish", "page": in.Page, "name": in.Name, "project": in.Project,
					"entry": orDefault(in.Entry, "index.html"), "files": files, "bytes": len(data)})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			p, err := c.PublishPage(e.in.Ctx, in, bytes.NewReader(data), int64(len(data)))
			if err != nil {
				return pageErr(err)
			}
			return e.p.Print(p, func(w io.Writer) {
				fmt.Fprintf(w, "published %s, version %d (%d files)\n%s\n", p.Name, p.Version, len(files), p.URL)
				fmt.Fprintln(w, "Write this address in your reply: the App opens it next to the chat.")
			})
		}),
	}
	list := &Command{
		Name:     "list",
		Summary:  "List the pages you can see",
		Examples: []string{"myco page list", "myco page list --project growth"},
		Flags:    []Flag{{Name: "project", Kind: String, Help: "Only this project's pages"}, adminFile},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			ps, err := c.Pages(e.in.Ctx, e.in.String("project"))
			if err != nil {
				return pageErr(err)
			}
			return e.p.Print(api.PageList{Pages: ps}, func(w io.Writer) {
				if len(ps) == 0 {
					fmt.Fprintln(w, "no pages")
					return
				}
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tVERSION\tPROJECT\tBY\tUPDATED")
				for _, p := range ps {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", p.ID, p.Name, p.Version, orDefault(p.Project, "-"), p.Publisher, p.UpdatedAt.Local().Format("2006-01-02 15:04"))
				}
				tw.Flush()
			})
		}),
	}
	get := &Command{
		Name:     "get",
		Summary:  "Show a page and its versions",
		Examples: []string{"myco page get --page pg_abc123"},
		Flags:    []Flag{pageFlag, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("page"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			p, err := c.Page(e.in.Ctx, e.in.String("page"))
			if err != nil {
				return pageErr(err)
			}
			return e.p.Print(p, func(w io.Writer) {
				fmt.Fprintf(w, "%s  %s\n%s\n", p.ID, p.Name, p.URL)
				if p.Project != "" {
					fmt.Fprintf(w, "project: %s\n", p.Project)
				}
				fmt.Fprintln(w, "versions (newest first):")
				for _, v := range p.Versions {
					fmt.Fprintf(w, "  %d  %s  %d files  %d bytes  by %s  opens on %s\n", v.N, v.At.Local().Format("2006-01-02 15:04"), v.Files, v.Size, v.By, v.Entry)
				}
			})
		}),
	}
	rm := &Command{
		Name:     "rm",
		Summary:  "Delete a page and all its versions",
		Mutating: true,
		Examples: []string{"myco page rm --page pg_abc123"},
		Flags:    []Flag{pageFlag, adminFile},
		Run: handler(func(e *env) error {
			if err := e.in.Require("page"); err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "page_rm", "page": e.in.String("page")})
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			if err := c.DeletePage(e.in.Ctx, e.in.String("page")); err != nil {
				return pageErr(err)
			}
			return e.p.Print(map[string]any{"deleted": e.in.String("page")}, func(w io.Writer) { fmt.Fprintln(w, "deleted") })
		}),
	}
	return (&Command{Name: "page", Summary: "Web pages people open in the App (reports, slides, prototypes)"}).Add(publish, list, get, rm)
}

// pageErr says what to do on a hub without pages.
func pageErr(err error) error {
	if e, ok := client.AsError(err); ok && e.Status == 404 && strings.Contains(e.Message, "not enabled") {
		return errorf("not_supported", ExitNotFound, "the hub needs v0.19.24 or later (sudo myco upgrade on the server); until then, send the files as attachments", "this hub has no pages yet")
	}
	return err
}

// skipInPage: what a page never needs.
func skipInPage(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__"
}

// zipPage zips a folder (or one html file) for publishing; it returns the
// zip, the files in it, and the entry when it is a single file.
func zipPage(dir string) ([]byte, []string, string, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, nil, "", errorf("invalid_argument", ExitUsage, "pass the folder of the page (or one .html file) to --dir", "%v", err)
	}
	type item struct{ rel, path string }
	var items []item
	entry := ""
	if !st.IsDir() {
		ext := strings.ToLower(filepath.Ext(dir))
		if ext != ".html" && ext != ".htm" {
			return nil, nil, "", errorf("invalid_argument", ExitUsage, "pass a folder, or a single .html file", "%s is not an html file", dir)
		}
		entry = filepath.Base(dir)
		items = append(items, item{entry, dir})
	} else {
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p != dir && skipInPage(d.Name()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			items = append(items, item{filepath.ToSlash(rel), p})
			return nil
		})
		if err != nil {
			return nil, nil, "", err
		}
	}
	if len(items) == 0 {
		return nil, nil, "", errorf("invalid_argument", ExitUsage, "put the page's files (index.html and what it uses) in the folder", "%s has no files to publish", dir)
	}
	if len(items) > 2000 {
		return nil, nil, "", errorf("invalid_argument", ExitUsage, "publish the built page (e.g. dist/), not its sources", "%s has %d files; a page has at most 2000", dir, len(items))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].rel < items[j].rel })
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	var names []string
	var htmls []string
	for _, it := range items {
		info, err := os.Stat(it.path)
		if err != nil {
			return nil, nil, "", err
		}
		if info.Size() > 20<<20 {
			return nil, nil, "", errorf("invalid_argument", ExitUsage, "keep large files (videos, data) out of the page, or make them smaller", "%s is larger than 20 MB", it.rel)
		}
		f, err := os.Open(it.path)
		if err != nil {
			return nil, nil, "", err
		}
		w, err := z.CreateHeader(&zip.FileHeader{Name: it.rel, Method: zip.Deflate, Modified: info.ModTime()})
		if err == nil {
			_, err = io.Copy(w, f)
		}
		f.Close()
		if err != nil {
			return nil, nil, "", err
		}
		names = append(names, it.rel)
		if e := strings.ToLower(filepath.Ext(it.rel)); e == ".html" || e == ".htm" {
			htmls = append(htmls, it.rel)
		}
		if buf.Len() > 50<<20 {
			return nil, nil, "", errorf("invalid_argument", ExitUsage, "leave out what the page does not need (sources, large videos)", "%s is larger than 50 MB zipped", dir)
		}
	}
	if err := z.Close(); err != nil {
		return nil, nil, "", err
	}
	if entry == "" && len(htmls) == 0 {
		return nil, nil, "", errorf("invalid_argument", ExitUsage, "a page opens on an html file, index.html by default", "%s has no html file", dir)
	}
	return buf.Bytes(), names, entry, nil
}
