package browser

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractArchive(archivePath, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name)
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator)) && targetAbs != destAbs {
			return fmt.Errorf("archive path traversal: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if filepath.IsAbs(hdr.Linkname) || strings.Contains(hdr.Linkname, "..") {
				continue
			}
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator)) && targetAbs != destAbs {
			return fmt.Errorf("archive path traversal: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func flattenSingleDir(destDir string) {
	entries, err := os.ReadDir(destDir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return
	}
	if strings.HasSuffix(entries[0].Name(), ".app") {
		return
	}
	sub := filepath.Join(destDir, entries[0].Name())
	children, err := os.ReadDir(sub)
	if err != nil {
		return
	}
	for _, c := range children {
		_ = os.Rename(filepath.Join(sub, c.Name()), filepath.Join(destDir, c.Name()))
	}
	_ = os.Remove(sub)
}
