//go:build windows

package main

import "golang.org/x/sys/windows"

func setupConsole() {
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
}
