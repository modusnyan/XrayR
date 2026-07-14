package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/XrayR-project/XrayR/api"
)

const Version = 1

type Rule struct {
	ID      int    `json:"id"`
	Pattern string `json:"pattern"`
}

type Payload struct {
	Version  int            `json:"version"`
	Identity string         `json:"identity"`
	SavedAt  time.Time      `json:"saved_at"`
	Node     api.NodeInfo   `json:"node"`
	Users    []api.UserInfo `json:"users"`
	Rules    []Rule         `json:"rules,omitempty"`
	Checksum string         `json:"checksum"`
}

type Store struct {
	directory string
	identity  string
	maxAge    time.Duration
}

func Identity(info api.ClientInfo) string {
	host := info.APIHost
	if parsed, err := url.Parse(info.APIHost); err == nil {
		host = strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", host, info.NodeID, strings.ToLower(info.NodeType))))
	return hex.EncodeToString(sum[:])
}

func New(directory string, info api.ClientInfo, maxAge time.Duration) *Store {
	return &Store{directory: directory, identity: Identity(info), maxAge: maxAge}
}

func (s *Store) Save(node *api.NodeInfo, users *[]api.UserInfo, rules *[]api.DetectRule) error {
	if node == nil || users == nil {
		return errors.New("node and users are required")
	}
	copyNode := *node
	if copyNode.REALITYConfig != nil {
		reality := *copyNode.REALITYConfig
		// Private keys are deliberately never persisted. Such a snapshot cannot
		// bootstrap local REALITY but still remains useful for diagnostics.
		reality.PrivateKey = ""
		copyNode.REALITYConfig = &reality
	}
	payload := Payload{Version: Version, Identity: s.identity, SavedAt: time.Now().UTC(), Node: copyNode, Users: append([]api.UserInfo(nil), (*users)...)}
	if rules != nil {
		for _, rule := range *rules {
			pattern := ""
			if rule.Pattern != nil {
				pattern = rule.Pattern.String()
			}
			payload.Rules = append(payload.Rules, Rule{ID: rule.ID, Pattern: pattern})
		}
	}
	checksum, err := payloadChecksum(payload)
	if err != nil {
		return err
	}
	payload.Checksum = checksum
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.directory, ".snapshot-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, s.path())
}

func (s *Store) Load() (*api.NodeInfo, *[]api.UserInfo, *[]api.DetectRule, time.Time, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, nil, nil, time.Time{}, err
	}
	var payload Payload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil, nil, time.Time{}, err
	}
	if payload.Version != Version || payload.Identity != s.identity {
		return nil, nil, nil, time.Time{}, errors.New("snapshot identity or version mismatch")
	}
	checksum := payload.Checksum
	payload.Checksum = ""
	expected, err := payloadChecksum(payload)
	if err != nil {
		return nil, nil, nil, time.Time{}, err
	}
	if checksum != expected {
		return nil, nil, nil, time.Time{}, errors.New("snapshot checksum mismatch")
	}
	if s.maxAge > 0 && time.Since(payload.SavedAt) > s.maxAge {
		return nil, nil, nil, time.Time{}, errors.New("snapshot is expired")
	}
	if payload.Node.Port == 0 || len(payload.Users) == 0 {
		return nil, nil, nil, time.Time{}, errors.New("snapshot is incomplete")
	}
	rules := make([]api.DetectRule, 0, len(payload.Rules))
	for _, rule := range payload.Rules {
		if rule.Pattern == "" {
			continue
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, nil, nil, time.Time{}, fmt.Errorf("invalid cached rule: %w", err)
		}
		rules = append(rules, api.DetectRule{ID: rule.ID, Pattern: compiled})
	}
	node := payload.Node
	users := payload.Users
	return &node, &users, &rules, payload.SavedAt, nil
}

func payloadChecksum(payload Payload) (string, error) {
	payload.Checksum = ""
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func (s *Store) path() string { return filepath.Join(s.directory, s.identity+".json") }
