package job

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CompletionEntry struct {
	Path  string
	Size  int64
	MTime time.Time
	Hash  string // empty if no hash is available
}

// CompletionLog is an append-only TSV log of completed files. Writes are
// buffered; callers should call Flush() or Close() to ensure durability.
// Format per line: <path>\t<size>\t<mtime_unix_nanos>\t<hash_or_dash>\n
type CompletionLog struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	buf      *bufio.Writer
	pending  int
	maxBatch int
}

const completionBatchSize = 100

func NewCompletionLog(path string) (*CompletionLog, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open completion log: %w", err)
	}
	return &CompletionLog{
		path:     path,
		file:     f,
		buf:      bufio.NewWriter(f),
		maxBatch: completionBatchSize,
	}, nil
}

func (c *CompletionLog) Append(e CompletionEntry) error {
	if strings.ContainsAny(e.Path, "\t\n") {
		return fmt.Errorf("path contains tab or newline: %q", e.Path)
	}
	hash := e.Hash
	if hash == "" {
		hash = "-"
	}
	line := fmt.Sprintf("%s\t%d\t%d\t%s\n", e.Path, e.Size, e.MTime.UnixNano(), hash)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.buf.WriteString(line); err != nil {
		return err
	}
	c.pending++
	if c.pending >= c.maxBatch {
		return c.flushLocked()
	}
	return nil
}

func (c *CompletionLog) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushLocked()
}

func (c *CompletionLog) flushLocked() error {
	if err := c.buf.Flush(); err != nil {
		return err
	}
	if err := c.file.Sync(); err != nil {
		return err
	}
	c.pending = 0
	return nil
}

func (c *CompletionLog) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file == nil {
		return nil
	}
	flushErr := c.flushLocked()
	closeErr := c.file.Close()
	c.file = nil
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

// LoadCompletionLog reads a log file into memory. Malformed lines are
// silently skipped. Missing files return an empty map.
func LoadCompletionLog(path string) (map[string]CompletionEntry, error) {
	result := make(map[string]CompletionEntry)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			entry, ok := parseCompletionLine(strings.TrimRight(line, "\n"))
			if ok {
				result[entry.Path] = entry
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseCompletionLine(line string) (CompletionEntry, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) != 4 {
		return CompletionEntry{}, false
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return CompletionEntry{}, false
	}
	mtimeNs, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return CompletionEntry{}, false
	}
	hash := parts[3]
	if hash == "-" {
		hash = ""
	}
	return CompletionEntry{
		Path:  parts[0],
		Size:  size,
		MTime: time.Unix(0, mtimeNs),
		Hash:  hash,
	}, true
}
