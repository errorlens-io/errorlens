package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// sourcemapsCmd implements:
//
//	errorlens sourcemaps upload --url https://errors.example.com --org acme --project web \
//	  --release web@1.2.3 --url-prefix "~/static/js" ./build/static/js
//
// The API token is read from --token or ERRORLENS_AUTH_TOKEN.
func sourcemapsCmd(args []string) error {
	if len(args) == 0 || args[0] != "upload" {
		return errors.New("usage: errorlens sourcemaps upload [flags] <dir>")
	}
	fs := flag.NewFlagSet("sourcemaps upload", flag.ContinueOnError)
	base := fs.String("url", os.Getenv("ERRORLENS_URL"), "ErrorLens server URL (or ERRORLENS_URL)")
	token := fs.String("token", os.Getenv("ERRORLENS_AUTH_TOKEN"), "API token (or ERRORLENS_AUTH_TOKEN)")
	org := fs.String("org", os.Getenv("ERRORLENS_ORG"), "organization slug (or ERRORLENS_ORG)")
	project := fs.String("project", os.Getenv("ERRORLENS_PROJECT"), "project slug (or ERRORLENS_PROJECT)")
	release := fs.String("release", os.Getenv("ERRORLENS_RELEASE"), "release version, must match the SDK's release (or ERRORLENS_RELEASE)")
	prefix := fs.String("url-prefix", "~/", `URL path the files are served under, e.g. "~/static/js"`)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 || *base == "" || *token == "" || *org == "" || *project == "" || *release == "" {
		fs.Usage()
		return errors.New("--url, --token, --org, --project, --release and a directory are required")
	}
	dir := fs.Arg(0)
	endpoint := fmt.Sprintf("%s/api/0/organizations/%s/projects/%s/releases/%s/files",
		strings.TrimRight(*base, "/"), url.PathEscape(*org), url.PathEscape(*project), url.PathEscape(*release))
	client := &http.Client{Timeout: 2 * time.Minute}

	count := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(p)
		if ext != ".js" && ext != ".map" && ext != ".mjs" && ext != ".cjs" {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		name := path.Join(*prefix, filepath.ToSlash(rel))
		if strings.HasPrefix(*prefix, "~/") && !strings.HasPrefix(name, "~/") {
			name = "~/" + strings.TrimPrefix(name, "~")
		}
		if err := uploadFile(client, endpoint, *token, p, name); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		fmt.Printf("  uploaded %s\n", name)
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no .js or .map files found in %s", dir)
	}
	fmt.Printf("Uploaded %d files for release %s.\n", count, *release)
	return nil
}

func uploadFile(c *http.Client, endpoint, token, file, name string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("name", name)
	part, err := mw.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	mw.Close()
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("server responded %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
