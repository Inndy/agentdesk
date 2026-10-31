package desk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	Endpoint   string `json:"endpoint"`
	PublicURL  string `json:"public_url"`
	AgentToken string `json:"agent_token"`
}
type HumanCredential struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// DataDir uses Git's shared directory so every worktree discovers the same project.
func DataDir(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("AGENTDESK_DATA_DIR")
	}
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err == nil {
		return filepath.Join(strings.TrimSpace(string(out)), "agentdesk"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".agentdesk", "project.json")); err == nil {
			return filepath.Join(dir, ".agentdesk"), nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return filepath.Join(cwd, ".agentdesk"), nil
}

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func LoadConfig(dir string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		return c, fmt.Errorf("project not initialized; run agentdesk init: %w", err)
	}
	err = json.Unmarshal(b, &c)
	if err == nil && (c.ProjectID == "" || c.AgentToken == "") {
		err = errors.New("invalid project configuration")
	}
	return c, err
}
func Init(dir, name string) (Config, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	// Initialization must not race another initializer or a running service.
	unlock, err := LockServer(dir)
	if err != nil {
		return Config{}, err
	}
	defer unlock()
	if c, err := LoadConfig(dir); err == nil {
		return c, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "project.json")); err == nil {
		return Config{}, errors.New("existing project configuration is invalid")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	if name == "" {
		cwd, _ := os.Getwd()
		name = filepath.Base(cwd)
	}
	c := Config{ProjectID: "P-" + RandomToken()[:16], Name: name, Endpoint: "http://127.0.0.1:7331", PublicURL: "http://127.0.0.1:7331", AgentToken: RandomToken()}
	h := HumanCredential{Token: RandomToken(), ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour)}
	if err := WriteJSON(filepath.Join(dir, "human.json"), h); err != nil {
		return c, err
	}
	return c, WriteJSON(filepath.Join(dir, "project.json"), c)
}
func LoadHuman(dir string) (HumanCredential, error) {
	var h HumanCredential
	b, err := os.ReadFile(filepath.Join(dir, "human.json"))
	if err != nil {
		return h, err
	}
	err = json.Unmarshal(b, &h)
	return h, err
}
