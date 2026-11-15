//go:build darwin

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The Mac's menu bar, in the language the App's pages show (redrawn when it
// changes): the usual app, edit and window menus (the system's own actions,
// relabelled), and myco's: settings, a new project, connecting a workspace
// or a computer, search, the sidebar, help. What the page does is asked for
// with a "myco-menu" event (AppMode).
func appMenu(app *application.App, zh trayText, send func(cmd string)) *application.Menu {
	m := app.NewMenu()
	do := func(cmd string) func(*application.Context) { return func(*application.Context) { send(cmd) } }
	role := func(sub *application.Menu, r application.Role, label string) {
		sub.AddRole(r)
		if it := sub.FindByRole(r); it != nil {
			it.SetLabel(label)
		}
	}
	open := func(url string) func(*application.Context) {
		return func(*application.Context) { _ = app.Browser.OpenURL(url) }
	}

	// myco
	my := m.AddSubmenu("myco")
	my.Add(zh.s("关于 myco", "About myco")).OnClick(do("about"))
	my.Add(zh.s("检查更新…", "Check for Updates…")).OnClick(do("about"))
	my.AddSeparator()
	my.Add(zh.s("设置…", "Settings…")).SetAccelerator("CmdOrCtrl+,").OnClick(do("settings"))
	my.AddSeparator()
	role(my, application.ServicesMenu, zh.s("服务", "Services"))
	my.AddSeparator()
	role(my, application.Hide, zh.s("隐藏 myco", "Hide myco"))
	role(my, application.HideOthers, zh.s("隐藏其他", "Hide Others"))
	role(my, application.ShowAll, zh.s("全部显示", "Show All"))
	my.AddSeparator()
	role(my, application.Quit, zh.s("退出 myco", "Quit myco"))

	// File
	file := m.AddSubmenu(zh.s("文件", "File"))
	file.Add(zh.s("新建项目…", "New Project…")).SetAccelerator("CmdOrCtrl+n").OnClick(do("new-project"))
	file.AddSeparator()
	file.Add(zh.s("连接工作区…", "Connect a Workspace…")).OnClick(do("connect"))
	file.Add(zh.s("连上一台电脑…", "Add a Computer…")).OnClick(do("add-computer"))
	file.AddSeparator()
	file.Add(zh.s("刷新", "Refresh")).SetAccelerator("CmdOrCtrl+r").OnClick(do("refresh"))
	file.AddSeparator()
	role(file, application.CloseWindow, zh.s("关闭窗口", "Close Window"))

	// Edit: the system's own (text fields, copying from the chat)
	edit := m.AddSubmenu(zh.s("编辑", "Edit"))
	role(edit, application.Undo, zh.s("撤销", "Undo"))
	role(edit, application.Redo, zh.s("重做", "Redo"))
	edit.AddSeparator()
	role(edit, application.Cut, zh.s("剪切", "Cut"))
	role(edit, application.Copy, zh.s("拷贝", "Copy"))
	role(edit, application.Paste, zh.s("粘贴", "Paste"))
	role(edit, application.SelectAll, zh.s("全选", "Select All"))

	// View
	view := m.AddSubmenu(zh.s("显示", "View"))
	view.Add(zh.s("搜索…", "Search…")).SetAccelerator("CmdOrCtrl+k").OnClick(do("search"))
	view.Add(zh.s("显示 / 隐藏侧栏", "Show / Hide Sidebar")).SetAccelerator("CmdOrCtrl+\\").OnClick(do("sidebar"))
	view.AddSeparator()
	role(view, application.ToggleFullscreen, zh.s("进入 / 退出全屏", "Enter / Exit Full Screen"))

	// Window
	win := m.AddSubmenu(zh.s("窗口", "Window"))
	role(win, application.Minimise, zh.s("最小化", "Minimize"))
	role(win, application.Zoom, zh.s("缩放", "Zoom"))
	win.AddSeparator()
	role(win, application.BringAllToFront, zh.s("前置全部窗口", "Bring All to Front"))

	// Help
	help := m.AddSubmenu(zh.s("帮助", "Help"))
	help.Add(zh.s("使用文档", "Documentation")).OnClick(open(zh.s("https://github.com/chenhg5/myco/blob/main/INSTALL.md", "https://github.com/chenhg5/myco#readme")))
	help.Add(zh.s("更新记录", "Release Notes")).OnClick(open("https://github.com/chenhg5/myco/releases"))
	help.AddSeparator()
	help.Add(zh.s("反馈问题…", "Report an Issue…")).OnClick(open("https://github.com/chenhg5/myco/issues/new"))
	return m
}
