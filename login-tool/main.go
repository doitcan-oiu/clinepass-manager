package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	setupConsole()
	xlsxFlag := flag.String("xlsx", "", "Excel 路径，默认读程序所在目录里最新的 xlsx")
	listOnly := flag.Bool("list", false, "只列出账号，不打开浏览器")
	flag.Parse()

	dir, err := workDir()
	if err != nil {
		fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		fatal(err)
	}

	fmt.Println("ClinePass 付款助手")
	fmt.Println("把支付链接 Excel 放到本程序同一目录，按行用 Cookie 打开 Chrome，付完再切下一条。")
	fmt.Println()

	path := strings.TrimSpace(*xlsxFlag)
	if path == "" {
		path, err = findExcel(dir)
		if err != nil {
			fmt.Printf("当前目录：%s\n", dir)
			fatal(err)
		}
	}
	rows, err := loadPayRows(path)
	if err != nil {
		fatal(err)
	}
	ready := readyRows(rows)
	fmt.Printf("已找到：%s\n共 %d 条，其中有 Cookie 的 %d 条\n\n", filepath.Base(path), len(rows), len(ready))
	if *listOnly {
		for i, row := range ready {
			fmt.Printf("%d. %s\n", i+1, displayEmail(row))
		}
		return
	}
	if len(ready) == 0 {
		fatal(fmt.Errorf("没有带 Cookie 的行，无法打开浏览器"))
	}

	in := bufio.NewReader(os.Stdin)
	fmt.Print("按回车开始，输入 q 退出：")
	if readQuit(in) {
		fmt.Println("已退出")
		return
	}

	chromePath, err := ensureChrome(filepath.Join(dir, "chrome"))
	if err != nil {
		fatal(err)
	}
	for i := 0; i < len(ready); i++ {
		row := ready[i]
		fmt.Printf("\n======== 第 %d/%d 条 ========\n", i+1, len(ready))
		fmt.Printf("账号：%s\n", displayEmail(row))
		fmt.Println("打开：https://app.cline.bot/dashboard")
		fmt.Println("正在打开 Chrome，请在浏览器里完成付款...")
		sess, err := openPayBrowser(chromePath, row)
		if err != nil {
			fmt.Printf("打开失败：%s\n", err)
			fmt.Print("按回车跳过这一条，输入 q 退出：")
			if quit := readQuit(in); quit {
				fmt.Println("已退出")
				return
			}
			continue
		}
		if i+1 < len(ready) {
			fmt.Print("付完后按回车：关闭当前浏览器并打开下一条。输入 q 退出：")
		} else {
			fmt.Print("这是最后一条。付完后按回车关闭浏览器，输入 q 退出：")
		}
		quit := readQuit(in)
		fmt.Println("正在关闭浏览器...")
		sess.Close()
		if quit {
			fmt.Println("已退出")
			return
		}
	}
	fmt.Println("\n全部处理完了。")
}

func readyRows(rows []payRow) []payRow {
	var out []payRow
	for _, row := range rows {
		if len(parseCookieHeader(row.Cookie)) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func displayEmail(row payRow) string {
	if row.Email != "" {
		return row.Email
	}
	return "(无账号)"
}

func workDir() (string, error) {
	cwd, err := os.Getwd()
	if err == nil {
		if _, findErr := findExcel(cwd); findErr == nil {
			return cwd, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err == nil {
		dir := filepath.Dir(exe)
		if !strings.Contains(exe, "go-build") {
			return dir, nil
		}
	}
	if cwd != "" {
		return cwd, nil
	}
	return "", fmt.Errorf("找不到工作目录")
}

func readQuit(in *bufio.Reader) bool {
	line, err := in.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return true
	}
	cmd := strings.ToLower(strings.TrimSpace(line))
	return cmd == "q" || cmd == "quit" || cmd == "exit"
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "错误：%s\n", err)
	os.Exit(1)
}
