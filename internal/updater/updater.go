package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repository = "ramaTImati/alldbackup"

const (
	maxReleaseInfo = 1 << 20
	maxChecksums   = 1 << 20
	maxArchive     = 128 << 20
	maxBinary      = 64 << 20
)

func Update(ctx context.Context, currentVersion string) (string, bool, error) {
	asset, err := releaseAsset()
	if err != nil {
		return "", false, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	info, err := get(ctx, client, "https://api.github.com/repos/"+repository+"/releases/latest", maxReleaseInfo, currentVersion)
	if err != nil {
		return "", false, fmt.Errorf("check latest release: %w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(info, &release); err != nil {
		return "", false, fmt.Errorf("parse latest release: %w", err)
	}
	if release.TagName == "" || strings.ContainsAny(release.TagName, `/\\`) {
		return "", false, errors.New("latest release has an invalid tag")
	}
	if currentVersion != "" && currentVersion != "dev" && strings.TrimPrefix(currentVersion, "v") == strings.TrimPrefix(release.TagName, "v") {
		return release.TagName, false, nil
	}

	baseURL := "https://github.com/" + repository + "/releases/download/" + url.PathEscape(release.TagName)
	checksums, err := get(ctx, client, baseURL+"/checksums.txt", maxChecksums, currentVersion)
	if err != nil {
		return "", false, fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := checksumFor(checksums, asset)
	if err != nil {
		return "", false, err
	}
	archive, err := get(ctx, client, baseURL+"/"+asset, maxArchive, currentVersion)
	if err != nil {
		return "", false, fmt.Errorf("download release archive: %w", err)
	}
	actual := sha256.Sum256(archive)
	if hex.EncodeToString(actual[:]) != expected {
		return "", false, errors.New("release archive checksum verification failed")
	}
	binary, err := releaseBinary(archive)
	if err != nil {
		return "", false, err
	}
	if err := replaceExecutable(binary); err != nil {
		return "", false, err
	}
	return release.TagName, true, nil
}

func releaseAsset() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("self-update supports Linux only (current OS: %s)", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "alldbackup_linux_" + runtime.GOARCH + ".tar.gz", nil
	default:
		return "", fmt.Errorf("self-update does not support Linux architecture %s", runtime.GOARCH)
	}
}

func get(ctx context.Context, client *http.Client, address string, limit int64, version string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "alldbackup/"+version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", address, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response from %s exceeds the size limit", address)
	}
	return data, nil
}

func checksumFor(data []byte, asset string) (string, error) {
	var checksum string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		if checksum != "" {
			return "", fmt.Errorf("release checksums contain multiple entries for %s", asset)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("release checksums contain an invalid SHA-256 for %s", asset)
		}
		checksum = strings.ToLower(fields[0])
	}
	if checksum == "" {
		return "", fmt.Errorf("release checksums do not include %s", asset)
	}
	return checksum, nil
}

func releaseBinary(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var binary []byte
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		if header.Name != "alldbackup" {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, errors.New("release archive alldbackup entry is not a regular file")
		}
		if binary != nil || header.Size <= 0 || header.Size > maxBinary {
			return nil, errors.New("release archive contains an invalid alldbackup binary")
		}
		binary, err = io.ReadAll(io.LimitReader(reader, maxBinary+1))
		if err != nil || int64(len(binary)) != header.Size {
			return nil, errors.New("release archive contains an incomplete alldbackup binary")
		}
		return binary, nil
	}
	return nil, errors.New("release archive does not contain alldbackup")
}

func replaceExecutable(binary []byte) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("inspect current executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("current executable %q is not a regular file", executable)
	}
	temp, err := os.CreateTemp(filepath.Dir(executable), ".alldbackup-update-*")
	if err != nil {
		return fmt.Errorf("create temporary update beside %q (run with sudo if this is a system install): %w", executable, err)
	}
	tempPath := temp.Name()
	defer func() {
		temp.Close()
		os.Remove(tempPath)
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("set updated executable permissions: %w", err)
	}
	if _, err := temp.Write(binary); err != nil {
		return fmt.Errorf("write updated executable: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync updated executable: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close updated executable: %w", err)
	}
	if err := os.Rename(tempPath, executable); err != nil {
		return fmt.Errorf("replace executable %q (run with sudo if this is a system install): %w", executable, err)
	}
	return nil
}
