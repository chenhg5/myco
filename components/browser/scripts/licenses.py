"""Collect notices for linked Go modules, without putting cache paths in output."""
import json
import pathlib
import subprocess
import sys

module_root = pathlib.Path(__file__).resolve().parents[1]
raw = subprocess.check_output(["go", "list", "-deps", "-json", "."], text=True, encoding="utf-8", cwd=module_root)
decoder = json.JSONDecoder()
modules = {}
while raw.strip():
    package, end = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[end:]
    module = package.get("Module", {})
    if module and not module.get("Main"):
        modules[module["Path"]] = module

notices = ["Third-party notices for the optional myco Browser Automation component\n"]
for name, module in sorted(modules.items()):
    directory = pathlib.Path(module["Dir"])
    files = [p for p in directory.iterdir() if p.is_file() and p.name.upper() in
             {"LICENSE", "LICENSE.TXT", "LICENSE.MD", "LICENCE", "COPYING"}]
    if not files:
        raise RuntimeError("Missing third-party license for " + name)
    notices.append("\n" + "=" * 72 + "\n" + name + " " + module.get("Version", "") + "\n")
    for file in sorted(files):
        notices.append(file.read_text(encoding="utf-8", errors="replace") + "\n")
# The recorder UI is adapted from BrowserWing, independently of Go modules.
# Existing component packaging already includes this combined notice on every OS.
notices.append("\n" + "=" * 72 + "\nBrowserWing recorder UI (MIT)\n")
notices.append((module_root / "NOTICE-BrowserWing.txt").read_text(encoding="utf-8"))
pathlib.Path(sys.argv[1]).write_text("".join(notices), encoding="utf-8")
