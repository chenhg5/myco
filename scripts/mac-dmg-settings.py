# dmgbuild settings for myco.dmg (scripts/mac-dmg.sh): the window is the
# background image (660 x 420 points, cmd/myco-app/build/darwin/dmg), the
# app's icon in its left slot, Applications in its right one, nothing else
# shown. Coordinates are icon centres in points from the window's top left;
# keep them in step with the slots in background.html.
import os.path

app = defines["app"]  # noqa: F821 (given by dmgbuild)
appname = os.path.basename(app)

format = "UDZO"
filesystem = "HFS+"
files = [app]
symlinks = {"Applications": "/Applications"}
hide_extensions = [appname]

background = defines["background"]  # noqa: F821
window_rect = ((240, 160), (660, 420))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_icon_preview = False

arrange_by = None
icon_size = 112
text_size = 13
label_pos = "bottom"
icon_locations = {
    appname: (180, 176),
    "Applications": (480, 176),
}
