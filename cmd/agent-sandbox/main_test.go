package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandFleetComposeFiles_NonFleet(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(composePath, []byte("services:\n  agent:\n    build: .\n"), 0644)

	result := expandFleetComposeFiles(dir, composePath)
	assert.Equal(t, []string{composePath}, result)
}

func TestExpandFleetComposeFiles_Fleet(t *testing.T) {
	dir := t.TempDir()

	// Create sub-compose files
	agent1Dir := filepath.Join(dir, "agent1")
	os.MkdirAll(agent1Dir, 0755)
	os.WriteFile(filepath.Join(agent1Dir, "docker-compose.yml"), []byte("services:\n  agent1:\n    build: .\n"), 0644)

	agent2Dir := filepath.Join(dir, "agent2")
	os.MkdirAll(agent2Dir, 0755)
	os.WriteFile(filepath.Join(agent2Dir, "docker-compose.yml"), []byte("services:\n  agent2:\n    build: .\n"), 0644)

	// Create fleet umbrella compose
	composePath := filepath.Join(dir, "docker-compose.yml")
	content := "include:\n  - agent1/docker-compose.yml\n  - agent2/docker-compose.yml\n"
	os.WriteFile(composePath, []byte(content), 0644)

	result := expandFleetComposeFiles(dir, composePath)
	assert.Equal(t, []string{
		filepath.Join(dir, "agent1", "docker-compose.yml"),
		filepath.Join(dir, "agent2", "docker-compose.yml"),
	}, result)
}

func TestExpandFleetComposeFiles_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	// Create a file outside buildDir
	outsideDir := t.TempDir()
	os.WriteFile(filepath.Join(outsideDir, "docker-compose.yml"), []byte("services:\n  evil:\n    build: .\n"), 0644)

	// Create fleet compose with path traversal
	composePath := filepath.Join(dir, "docker-compose.yml")
	// Use relative path that escapes buildDir
	content := "include:\n  - ../../" + filepath.Base(outsideDir) + "/docker-compose.yml\n"
	os.WriteFile(composePath, []byte(content), 0644)

	result := expandFleetComposeFiles(dir, composePath)
	// Should fall back to original since traversal paths are excluded
	assert.Equal(t, []string{composePath}, result)
}

func TestExpandFleetComposeFiles_MissingFiles(t *testing.T) {
	dir := t.TempDir()

	// Create only one of two referenced files
	agent1Dir := filepath.Join(dir, "agent1")
	os.MkdirAll(agent1Dir, 0755)
	os.WriteFile(filepath.Join(agent1Dir, "docker-compose.yml"), []byte("services:\n  agent1:\n    build: .\n"), 0644)

	composePath := filepath.Join(dir, "docker-compose.yml")
	content := "include:\n  - agent1/docker-compose.yml\n  - agent2/docker-compose.yml\n"
	os.WriteFile(composePath, []byte(content), 0644)

	result := expandFleetComposeFiles(dir, composePath)
	// Only the existing file is returned
	assert.Equal(t, []string{
		filepath.Join(dir, "agent1", "docker-compose.yml"),
	}, result)
}
