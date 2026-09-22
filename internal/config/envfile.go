package config

import (
	"bufio"
	"os"
	"strings"
)

// loadEnvFiles loads dotenv-style files. Keys already set in the process environment
// (e.g. PowerShell $env:MODEL) are left unchanged. Duplicate keys in one file: last wins.
func loadEnvFiles() {
	paths := []string{os.Getenv("ENV_FILE"), "configs/.env", ".env"}
	for _, p := range paths {
		if p == "" {
			continue
		}
		loadEnvFile(p)
	}
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	vars := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}
		vars[key] = val
	}
	for key, val := range vars {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}
