package downloader

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/time/rate"
)

type limitedReader struct {
	reader  io.Reader
	limiter *rate.Limiter
}

func (r *limitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		if waitErr := r.limiter.WaitN(context.Background(), n); waitErr != nil {
			return 0, waitErr
		}
	}
	return n, err
}

type DownloadResult struct {
	BytesWritten int64
	Filename     string
}

func DownloadFile(ctx context.Context, downloadURL, destPath string, limitKBps int, agentUUID, secretKey string) (int64, error) {
	res, err := DownloadFileWithMeta(ctx, downloadURL, destPath, limitKBps, agentUUID, secretKey)
	if err != nil {
		return 0, err
	}
	return res.BytesWritten, nil
}

func DownloadFileWithMeta(
	ctx context.Context,
	downloadURL,
	destPath string,
	limitKBps int,
	agentUUID,
	secretKey string,
) (*DownloadResult, error) {
	if limitKBps <= 0 {
		return nil, fmt.Errorf("invalid bandwidth limit: %d", limitKBps)
	}

	resumeOffset := int64(0)
	if fi, err := os.Stat(destPath); err == nil {
		resumeOffset = fi.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-UUID", agentUUID)
	req.Header.Set("X-Agent-Secret", secretKey)
	if resumeOffset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(resumeOffset, 10)+"-")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("download request failed: %s", resp.Status)
	}

	openFlags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent {
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}

	out, err := os.OpenFile(destPath, openFlags, 0o644)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	limiter := rate.NewLimiter(rate.Limit(limitKBps*1024), limitKBps*1024)
	lr := &limitedReader{
		reader:  resp.Body,
		limiter: limiter,
	}

	n, err := io.Copy(out, lr)
	if err != nil {
		return nil, err
	}
	return &DownloadResult{
		BytesWritten: n,
		Filename:     extractFilename(resp.Header.Get("Content-Disposition"), destPath),
	}, nil
}

func extractFilename(contentDisposition, fallbackPath string) string {
	if contentDisposition != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil {
			if filename := strings.TrimSpace(params["filename"]); filename != "" {
				return filename
			}
		}
	}
	return filepath.Base(fallbackPath)
}
