package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultUpdateAPI = "https://api.github.com/repos/gongahkia/gator/releases/latest"

const maxUpdateArchiveBytes = 128 * 1024 * 1024

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseUpdater struct {
	Client     *http.Client
	APIURL     string
	Executable func() (string, error)
	GOOS       string
	GOARCH     string
	Version    string
}

func update(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	checkOnly := flags.Bool("check", false, "report whether a newer release is available without installing it")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator update [--check]")
	}
	updater := defaultUpdater()
	latest, err := updater.latest()
	if err != nil {
		return err
	}
	newer, err := newerVersion(version, latest.TagName)
	if err != nil {
		return err
	}
	if !newer {
		_, err := fmt.Fprintf(out, "Gator %s is up to date.\n", version)
		return err
	}
	if *checkOnly {
		_, err := fmt.Fprintf(out, "Gator %s is available; current version is %s.\n", latest.TagName, version)
		return err
	}
	if version == "dev" {
		return fmt.Errorf("a development build cannot update itself; install %s with the documented install command", latest.TagName)
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Gator %s is available; Windows cannot replace a running executable, so reinstall with the documented install command", latest.TagName)
	}
	if err := updater.install(latest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated Gator from %s to %s. Restart Gator to use the new version.\n", version, latest.TagName)
	return err
}

func defaultUpdater() releaseUpdater {
	apiURL := os.Getenv("GATOR_UPDATE_API_URL")
	if apiURL == "" {
		apiURL = defaultUpdateAPI
	}
	return releaseUpdater{
		Client:     &http.Client{Timeout: 30 * time.Second},
		APIURL:     apiURL,
		Executable: os.Executable,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Version:    version,
	}
}

func (u releaseUpdater) latest() (release, error) {
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequest(http.MethodGet, u.APIURL, nil)
	if err != nil {
		return release{}, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "gator/"+u.Version)
	response, err := client.Do(request)
	if err != nil {
		return release{}, fmt.Errorf("check for Gator updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return release{}, errors.New("no published Gator release is available yet")
	}
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("check for Gator updates: server returned HTTP %d", response.StatusCode)
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&latest); err != nil {
		return release{}, fmt.Errorf("decode Gator update metadata: %w", err)
	}
	if _, err := parseVersion(latest.TagName); err != nil {
		return release{}, fmt.Errorf("latest Gator release has an invalid tag: %w", err)
	}
	return latest, nil
}

func (u releaseUpdater) install(latest release) error {
	if u.GOOS == "" || u.GOARCH == "" {
		return errors.New("update platform is required")
	}
	archiveName := archiveName(u.GOOS, u.GOARCH)
	archive, found := findAsset(latest.Assets, archiveName)
	if !found {
		return fmt.Errorf("Gator %s has no release archive for %s/%s", latest.TagName, u.GOOS, u.GOARCH)
	}
	checksums, found := findAsset(latest.Assets, "checksums.txt")
	if !found {
		return errors.New("Gator release does not include checksums.txt")
	}
	expected, err := u.downloadChecksum(checksums.URL, archiveName)
	if err != nil {
		return err
	}
	payload, err := u.download(archive.URL)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if !strings.EqualFold(expected, hex.EncodeToString(digest[:])) {
		return errors.New("Gator release checksum did not match; update was not installed")
	}
	binary, err := extractBinary(payload, archiveName, u.GOOS)
	if err != nil {
		return err
	}
	executable := u.Executable
	if executable == nil {
		executable = os.Executable
	}
	target, err := executable()
	if err != nil {
		return fmt.Errorf("locate current Gator executable: %w", err)
	}
	if err := replaceExecutable(target, binary); err != nil {
		return err
	}
	return nil
}

func (u releaseUpdater) downloadChecksum(url, archiveName string) (string, error) {
	contents, err := u.download(url)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != archiveName {
			continue
		}
		if len(fields[0]) != sha256.Size*2 {
			break
		}
		if _, err := hex.DecodeString(fields[0]); err == nil {
			return fields[0], nil
		}
		break
	}
	return "", fmt.Errorf("checksums.txt does not contain a SHA-256 checksum for %s", archiveName)
}

func (u releaseUpdater) download(url string) ([]byte, error) {
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create release download request: %w", err)
	}
	request.Header.Set("User-Agent", "gator/"+u.Version)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download Gator release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download Gator release: server returned HTTP %d", response.StatusCode)
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxUpdateArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Gator release: %w", err)
	}
	if len(contents) > maxUpdateArchiveBytes {
		return nil, fmt.Errorf("Gator release exceeds the %d MiB update limit", maxUpdateArchiveBytes/(1024*1024))
	}
	return contents, nil
}

func archiveName(goos, goarch string) string {
	name := "gator_" + goos + "_" + goarch
	if goos == "windows" {
		return name + ".zip"
	}
	return name + ".tar.gz"
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name && asset.URL != "" {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func extractBinary(contents []byte, archiveName, goos string) ([]byte, error) {
	binaryName := "gator"
	if goos == "windows" {
		binaryName += ".exe"
	}
	if strings.HasSuffix(archiveName, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
		if err != nil {
			return nil, fmt.Errorf("read Gator release archive: %w", err)
		}
		for _, entry := range reader.File {
			if filepath.Base(entry.Name) != binaryName || entry.FileInfo().IsDir() {
				continue
			}
			file, err := entry.Open()
			if err != nil {
				return nil, fmt.Errorf("open Gator binary in release archive: %w", err)
			}
			defer file.Close()
			return readUpdateBinary(file)
		}
		return nil, errors.New("Gator release archive does not contain the expected binary")
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(contents))
	if err != nil {
		return nil, fmt.Errorf("read Gator release archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read Gator release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		return readUpdateBinary(reader)
	}
	return nil, errors.New("Gator release archive does not contain the expected binary")
}

func readUpdateBinary(reader io.Reader) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, maxUpdateArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Gator binary from release archive: %w", err)
	}
	if len(contents) == 0 || len(contents) > maxUpdateArchiveBytes {
		return nil, errors.New("Gator binary in release archive is invalid")
	}
	return contents, nil
}

func replaceExecutable(target string, binary []byte) error {
	directory := filepath.Dir(target)
	temporary, err := os.CreateTemp(directory, ".gator-update-*")
	if err != nil {
		return fmt.Errorf("create replacement executable: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o755); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set replacement executable permissions: %w", err)
	}
	if _, err := temporary.Write(binary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write replacement executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close replacement executable: %w", err)
	}
	backup := target + ".previous"
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("prepare current executable for replacement: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			return fmt.Errorf("replace Gator executable: %w; restore previous executable: %v", err, restoreErr)
		}
		return fmt.Errorf("replace Gator executable: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("remove previous Gator executable: %w", err)
	}
	return nil
}

type semanticVersion struct{ major, minor, patch int }

func newerVersion(current, candidate string) (bool, error) {
	if current == "dev" {
		return true, nil
	}
	left, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("current Gator version is invalid: %w", err)
	}
	right, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("available Gator version is invalid: %w", err)
	}
	if right.major != left.major {
		return right.major > left.major, nil
	}
	if right.minor != left.minor {
		return right.minor > left.minor, nil
	}
	return right.patch > left.patch, nil
}

func parseVersion(value string) (semanticVersion, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("%q is not vMAJOR.MINOR.PATCH", value)
	}
	var parsed semanticVersion
	if _, err := fmt.Sscanf(value, "%d.%d.%d", &parsed.major, &parsed.minor, &parsed.patch); err != nil || parsed.major < 0 || parsed.minor < 0 || parsed.patch < 0 {
		return semanticVersion{}, fmt.Errorf("%q is not vMAJOR.MINOR.PATCH", value)
	}
	if fmt.Sprintf("%d.%d.%d", parsed.major, parsed.minor, parsed.patch) != value {
		return semanticVersion{}, fmt.Errorf("%q is not vMAJOR.MINOR.PATCH", value)
	}
	return parsed, nil
}
