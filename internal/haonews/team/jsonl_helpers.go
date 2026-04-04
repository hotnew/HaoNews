package team

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

func readLastJSONLLines(path string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return nil, nil
	}
	lines := make([]string, 0, limit)
	remaining := make([]byte, 0, 8192)
	for offset := info.Size(); offset > 0 && len(lines) < limit; {
		readSize := int64(8192)
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize
		buf := make([]byte, readSize)
		if _, err := file.ReadAt(buf, offset); err != nil {
			return nil, err
		}
		if len(remaining) > 0 {
			buf = append(buf, remaining...)
			remaining = remaining[:0]
		}
		for i := len(buf) - 1; i >= 0 && len(lines) < limit; i-- {
			if buf[i] != '\n' {
				continue
			}
			if line := strings.TrimSpace(string(buf[i+1:])); line != "" {
				lines = append(lines, line)
			}
			buf = buf[:i]
		}
		if len(buf) > 0 {
			remaining = append(remaining[:0], buf...)
		}
	}
	if len(lines) < limit {
		if line := strings.TrimSpace(string(remaining)); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func countNonEmptyJSONLLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func latestMessageTimestampFromJSONL(path string) (time.Time, error) {
	lines, err := readLastJSONLLines(path, 1)
	if err != nil {
		return time.Time{}, err
	}
	if len(lines) == 0 {
		return time.Time{}, nil
	}
	var msg Message
	if err := json.Unmarshal([]byte(lines[0]), &msg); err != nil {
		logTeamEvent("corrupt_jsonl_line", "path", path, "error", err)
		return time.Time{}, nil
	}
	return msg.CreatedAt, nil
}
