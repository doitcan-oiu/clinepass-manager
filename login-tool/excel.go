package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type payRow struct {
	Email    string
	Password string
	Cookie   string
	URL      string
}

func findExcel(dir string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod time.Time
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, "~$") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".xlsx") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best = filepath.Join(dir, name)
			bestMod = info.ModTime()
		}
	}
	if best == "" {
		return "", fmt.Errorf("目录里没有 xlsx：%s", dir)
	}
	return best, nil
}

func loadPayRows(path string) ([]payRow, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开 Excel 失败: %w", err)
	}
	defer f.Close()

	sheet := preferredSheet(f)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Excel 是空的")
	}
	col := mapHeaders(rows[0])
	if col["cookie"] < 0 && col["账号"] < 0 && col["email"] < 0 {
		return nil, fmt.Errorf("找不到表头（需要：账号、Cookie、支付链接）")
	}
	var out []payRow
	for _, cells := range rows[1:] {
		row := payRow{
			Email:    cell(cells, firstCol(col, "账号", "email", "帐号")),
			Password: cell(cells, firstCol(col, "密码", "password")),
			Cookie:   cell(cells, firstCol(col, "cookie")),
			URL:      cell(cells, firstCol(col, "支付链接", "付款链接", "url", "payment")),
		}
		if strings.TrimSpace(row.Cookie) == "" && strings.TrimSpace(row.URL) == "" && strings.TrimSpace(row.Email) == "" {
			continue
		}
		out = append(out, row)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Excel 里没有数据行")
	}
	return out, nil
}

func preferredSheet(f *excelize.File) string {
	for _, name := range f.GetSheetList() {
		if name == "支付链接" {
			return name
		}
	}
	return f.GetSheetName(0)
}

func mapHeaders(row []string) map[string]int {
	out := map[string]int{}
	for i, h := range row {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		out[key] = i
	}
	return out
}

func firstCol(cols map[string]int, names ...string) int {
	for _, name := range names {
		if i, ok := cols[strings.ToLower(name)]; ok {
			return i
		}
	}
	return -1
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
